package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/tyemirov/llm-proxy/internal/constants"
	"github.com/tyemirov/llm-proxy/internal/utils"
)

const (
	chatRoleSystem    chatRole = "system"
	chatRoleUser      chatRole = "user"
	chatRoleAssistant chatRole = "assistant"
)

type chatRole string

type chatMessagePayload struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	Order   *int   `json:"order,omitempty"`
}

type chatV2MessagePayload struct {
	Role        string          `json:"role"`
	Content     string          `json:"content"`
	Attachments json.RawMessage `json:"attachments"`
	Order       *int            `json:"order,omitempty"`
}

type chatMessage struct {
	role              chatRole
	content           string
	attachments       []messageMedia
	order             *int
	visibleInResponse bool
}

type chatMessages []chatMessage

type chatMessageCandidate struct {
	role        string
	content     string
	attachments []messageMedia
	order       *int
}

func newPromptChatMessages(userPrompt string, systemPrompt string, systemPromptVisibleInResponse bool) (chatMessages, error) {
	if userPrompt == constants.EmptyString {
		return nil, fmt.Errorf("%w: missing prompt", ErrInvalidChatMessages)
	}
	messages := chatMessages{}
	if !utils.IsBlank(systemPrompt) {
		messages = append(messages, chatMessage{role: chatRoleSystem, content: systemPrompt, visibleInResponse: systemPromptVisibleInResponse})
	}
	messages = append(messages, chatMessage{role: chatRoleUser, content: userPrompt, visibleInResponse: true})
	return messages, nil
}

func newPayloadChatMessages(payloadMessages []chatMessagePayload, defaultSystemPrompt string, requestSystemPrompt string) (chatMessages, error) {
	candidates := make([]chatMessageCandidate, 0, len(payloadMessages))
	for _, payloadMessage := range payloadMessages {
		candidates = append(candidates, chatMessageCandidate{
			role:    payloadMessage.Role,
			content: payloadMessage.Content,
			order:   payloadMessage.Order,
		})
	}
	return newCandidateChatMessages(candidates, defaultSystemPrompt, requestSystemPrompt)
}

func newV2PayloadChatMessages(payloadMessages []chatV2MessagePayload, defaultSystemPrompt string) (chatMessages, error) {
	candidates := make([]chatMessageCandidate, 0, len(payloadMessages))
	for messageIndex, payloadMessage := range payloadMessages {
		attachmentPayloads, attachmentsError := decodeChatMessageAttachments(payloadMessage.Attachments)
		if attachmentsError != nil {
			return nil, fmt.Errorf("messages[%d].attachments: %w", messageIndex, attachmentsError)
		}
		attachments := make([]messageMedia, 0, len(attachmentPayloads))
		for attachmentIndex, attachmentPayload := range attachmentPayloads {
			attachment, attachmentError := newMessageMedia(attachmentPayload)
			if attachmentError != nil {
				return nil, fmt.Errorf("messages[%d].attachments[%d]: %w", messageIndex, attachmentIndex, attachmentError)
			}
			attachments = append(attachments, attachment)
		}
		candidates = append(candidates, chatMessageCandidate{
			role:        payloadMessage.Role,
			content:     payloadMessage.Content,
			attachments: attachments,
			order:       payloadMessage.Order,
		})
	}
	return newCandidateChatMessages(candidates, defaultSystemPrompt, constants.EmptyString)
}

func decodeChatMessageAttachments(rawAttachments json.RawMessage) ([]chatMessageAttachmentPayload, error) {
	if rawAttachments == nil {
		return nil, nil
	}
	if bytes.Equal(bytes.TrimSpace(rawAttachments), []byte("null")) {
		return nil, fmt.Errorf("%w: attachments must be an array", ErrInvalidChatMessages)
	}
	var attachments []chatMessageAttachmentPayload
	decoder := json.NewDecoder(bytes.NewReader(rawAttachments))
	decoder.DisallowUnknownFields()
	if decodeError := decoder.Decode(&attachments); decodeError != nil {
		return nil, fmt.Errorf("%w: invalid attachments: %v", ErrInvalidChatMessages, decodeError)
	}
	if len(attachments) == 0 {
		return nil, fmt.Errorf("%w: attachments is empty", ErrInvalidChatMessages)
	}
	return attachments, nil
}

func newCandidateChatMessages(candidates []chatMessageCandidate, defaultSystemPrompt string, requestSystemPrompt string) (chatMessages, error) {
	if len(candidates) == 0 {
		return nil, fmt.Errorf("%w: empty messages", ErrInvalidChatMessages)
	}
	orderedCandidates, orderError := sortChatMessageCandidatesByOrder(candidates)
	if orderError != nil {
		return nil, orderError
	}
	messages := chatMessages{}
	hasSystemMessage := false
	hasUserMessage := false
	for messageIndex, candidate := range orderedCandidates {
		role, roleError := newChatRole(candidate.role)
		if roleError != nil {
			return nil, fmt.Errorf("%w: messages[%d].role", roleError, messageIndex)
		}
		if utils.IsBlank(candidate.content) {
			return nil, fmt.Errorf("%w: messages[%d].content is empty", ErrInvalidChatMessages, messageIndex)
		}
		if len(candidate.attachments) > 0 && role != chatRoleUser {
			return nil, fmt.Errorf("%w: messages[%d].attachments require user role", ErrInvalidChatMessages, messageIndex)
		}
		if role == chatRoleSystem {
			hasSystemMessage = true
		}
		if role == chatRoleUser {
			hasUserMessage = true
		}
		messages = append(messages, chatMessage{
			role:              role,
			content:           candidate.content,
			attachments:       candidate.attachments,
			order:             copyChatMessageOrder(candidate.order),
			visibleInResponse: true,
		})
	}
	if !hasUserMessage {
		return nil, fmt.Errorf("%w: messages must include a user message", ErrInvalidChatMessages)
	}
	if !utils.IsBlank(requestSystemPrompt) {
		if hasSystemMessage {
			return nil, fmt.Errorf("%w: system_prompt conflicts with messages[].role=system", ErrInvalidChatMessages)
		}
		return append(chatMessages{{role: chatRoleSystem, content: requestSystemPrompt, visibleInResponse: true}}, messages...), nil
	}
	if !hasSystemMessage && !utils.IsBlank(defaultSystemPrompt) {
		return append(chatMessages{{role: chatRoleSystem, content: defaultSystemPrompt}}, messages...), nil
	}
	return messages, nil
}

func sortChatMessageCandidatesByOrder(candidates []chatMessageCandidate) ([]chatMessageCandidate, error) {
	orderedCandidates := append([]chatMessageCandidate(nil), candidates...)
	hasExplicitOrder := false
	for _, candidate := range orderedCandidates {
		if candidate.order != nil {
			hasExplicitOrder = true
			break
		}
	}
	if !hasExplicitOrder {
		return orderedCandidates, nil
	}
	seenOrders := map[int]struct{}{}
	for messageIndex, candidate := range orderedCandidates {
		if candidate.order == nil {
			return nil, fmt.Errorf("%w: messages[%d].order missing", ErrInvalidChatMessages, messageIndex)
		}
		if *candidate.order < 0 {
			return nil, fmt.Errorf("%w: messages[%d].order is negative", ErrInvalidChatMessages, messageIndex)
		}
		if _, exists := seenOrders[*candidate.order]; exists {
			return nil, fmt.Errorf("%w: duplicate messages[].order=%d", ErrInvalidChatMessages, *candidate.order)
		}
		seenOrders[*candidate.order] = struct{}{}
	}
	sort.SliceStable(orderedCandidates, func(firstIndex int, secondIndex int) bool {
		return *orderedCandidates[firstIndex].order < *orderedCandidates[secondIndex].order
	})
	return orderedCandidates, nil
}

func copyChatMessageOrder(order *int) *int {
	if order == nil {
		return nil
	}
	copiedOrder := *order
	return &copiedOrder
}

func newChatRole(rawRole string) (chatRole, error) {
	normalizedRole := strings.ToLower(strings.TrimSpace(rawRole))
	switch normalizedRole {
	case string(chatRoleSystem):
		return chatRoleSystem, nil
	case string(chatRoleUser):
		return chatRoleUser, nil
	case string(chatRoleAssistant):
		return chatRoleAssistant, nil
	default:
		return constants.EmptyString, fmt.Errorf("%w: unsupported role=%s", ErrInvalidChatMessages, rawRole)
	}
}

func (messages chatMessages) openAIResponsesInput() string {
	if len(messages) == 1 && messages[0].role == chatRoleUser {
		return messages[0].content
	}
	if len(messages) == 2 && messages[0].role == chatRoleSystem && messages[1].role == chatRoleUser {
		return messages[0].content + "\n\n" + messages[1].content
	}
	var transcriptBuilder strings.Builder
	for messageIndex, message := range messages {
		if messageIndex > 0 {
			transcriptBuilder.WriteString("\n\n")
		}
		transcriptBuilder.WriteString(string(message.role))
		transcriptBuilder.WriteString(":\n")
		transcriptBuilder.WriteString(message.content)
	}
	return transcriptBuilder.String()
}

func (messages chatMessages) requestDisplayText() string {
	return messages.responseVisibleMessages().openAIResponsesInput()
}

func (messages chatMessages) responseRequestMessages() []map[string]any {
	visibleMessages := messages.responseVisibleMessages()
	responseMessages := make([]map[string]any, 0, len(visibleMessages))
	for _, message := range visibleMessages {
		responseMessage := map[string]any{
			"role":    string(message.role),
			"content": message.content,
		}
		if message.order != nil {
			responseMessage["order"] = *message.order
		}
		responseMessages = append(responseMessages, responseMessage)
	}
	return responseMessages
}

func (messages chatMessages) responseVisibleMessages() chatMessages {
	visibleMessages := make(chatMessages, 0, len(messages))
	for _, message := range messages {
		if message.visibleInResponse {
			visibleMessages = append(visibleMessages, message)
		}
	}
	return visibleMessages
}
