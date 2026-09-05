package proxy

import (
	"bytes"
	"encoding/base64"
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
	ToolCalls   []functionCall  `json:"tool_calls,omitempty"`
	ToolCallID  string          `json:"tool_call_id,omitempty"`
	Role        string          `json:"role"`
	Content     string          `json:"content"`
	Attachments json.RawMessage `json:"attachments"`
	Order       *int            `json:"order,omitempty"`
}

type chatMessage struct {
	toolCalls         []functionCall
	toolCallID        string
	role              chatRole
	content           string
	attachments       []messageMedia
	order             *int
	visibleInResponse bool
}

type chatMessages []chatMessage

type chatMessageCandidate struct {
	toolCalls   []functionCall
	toolCallID  string
	role        string
	content     string
	attachments []messageMedia
	order       *int
}

func validateMessagesForResolvedTextRoute(model textModelDefinition, messages chatMessages) error {
	if model.wireContract != textWireContractGeminiInteractions {
		return nil
	}
	for _, message := range messages {
		if message.role == chatRoleAssistant {
			return fmt.Errorf("%w: Gemini Interactions assistant history is unsupported", ErrInvalidChatMessages)
		}
	}
	return nil
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

func newV2PayloadChatMessages(payloadMessages []chatV2MessagePayload, defaultSystemPrompt string, requestTenant tenant, assetStore *tenantAssetStore) (messages chatMessages, messagesError error) {
	candidates := make([]chatMessageCandidate, 0, len(payloadMessages))
	defer func() {
		if messagesError != nil {
			chatMessagesFromCandidates(candidates).closeMedia()
		}
	}()
	for messageIndex, payloadMessage := range payloadMessages {
		attachmentPayloads, attachmentsError := decodeChatMessageAttachments(payloadMessage.Attachments)
		if attachmentsError != nil {
			return nil, fmt.Errorf("messages[%d].attachments: %w", messageIndex, attachmentsError)
		}
		attachments := make([]messageMedia, 0, len(attachmentPayloads))
		for attachmentIndex, attachmentPayload := range attachmentPayloads {
			attachment, attachmentError := newMessageMedia(attachmentPayload, requestTenant, assetStore)
			if attachmentError != nil {
				candidates = append(candidates, chatMessageCandidate{attachments: attachments})
				return nil, fmt.Errorf("messages[%d].attachments[%d]: %w", messageIndex, attachmentIndex, attachmentError)
			}
			attachments = append(attachments, attachment)
		}
		candidates = append(candidates, chatMessageCandidate{
			toolCalls:   payloadMessage.ToolCalls,
			toolCallID:  payloadMessage.ToolCallID,
			role:        payloadMessage.Role,
			content:     payloadMessage.Content,
			attachments: attachments,
			order:       payloadMessage.Order,
		})
	}
	return newCandidateChatMessages(candidates, defaultSystemPrompt, constants.EmptyString)
}

func chatMessagesFromCandidates(candidates []chatMessageCandidate) chatMessages {
	messages := make(chatMessages, 0, len(candidates))
	for _, candidate := range candidates {
		messages = append(messages, chatMessage{attachments: candidate.attachments})
	}
	return messages
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
		if utils.IsBlank(candidate.content) && len(candidate.toolCalls) == 0 && role != chatRoleTool {
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
			toolCalls:         candidate.toolCalls,
			toolCallID:        candidate.toolCallID,
			role:              role,
			content:           candidate.content,
			attachments:       candidate.attachments,
			order:             copyChatMessageOrder(candidate.order),
			visibleInResponse: true,
		})
	}
	if err := validateToolHistory(messages); err != nil {
		return nil, err
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
	case string(chatRoleTool):
		return chatRoleTool, nil
	case string(chatRoleAssistant):
		return chatRoleAssistant, nil
	default:
		return constants.EmptyString, fmt.Errorf("%w: unsupported role=%s", ErrInvalidChatMessages, rawRole)
	}
}

func (messages chatMessages) openAIResponsesTextInput() string {
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

func (messages chatMessages) openAIResponsesInput(imageDetail string, preserveRoles bool) (any, error) {
	if messages.mediaCount() == 0 && !preserveRoles && !messages.hasToolHistory() {
		return messages.openAIResponsesTextInput(), nil
	}
	providerMessages := make([]map[string]any, 0, len(messages))
	for messageIndex := range messages {
		message := &messages[messageIndex]
		if message.role == chatRoleTool {
			providerMessages = append(providerMessages, map[string]any{"type": "function_call_output", "call_id": message.toolCallID, "output": message.content})
			continue
		}
		if len(message.toolCalls) > 0 {
			for _, call := range message.toolCalls {
				providerMessages = append(providerMessages, map[string]any{"type": "function_call", "call_id": call.ID, "name": call.Name, "arguments": call.Arguments})
			}
			if message.content == "" {
				continue
			}
		}
		if len(message.attachments) == 0 {
			providerMessages = append(providerMessages, map[string]any{"role": string(message.role), "content": message.content})
			continue
		}
		content := make([]map[string]any, 0, len(message.attachments)+1)
		for attachmentIndex := range message.attachments {
			attachment := &message.attachments[attachmentIndex]
			data, dataError := attachment.bytes()
			if dataError != nil {
				return nil, dataError
			}
			content = append(content, map[string]any{
				"type":      "input_image",
				"image_url": "data:" + attachment.mimeType + ";base64," + base64.StdEncoding.EncodeToString(data),
				"detail":    imageDetail,
			})
		}
		content = append(content, map[string]any{"type": "input_text", "text": message.content})
		providerMessages = append(providerMessages, map[string]any{"role": string(message.role), "content": content})
	}
	return providerMessages, nil
}

func (messages chatMessages) chatCompletionMessages() ([]chatCompletionMessage, error) {
	providerMessages := make([]chatCompletionMessage, 0, len(messages))
	for messageIndex := range messages {
		message := &messages[messageIndex]
		if len(message.attachments) == 0 {
			providerMessages = append(providerMessages, chatCompletionMessage{Role: string(message.role), Content: message.content, ToolCalls: chatFunctionCalls(message.toolCalls), ToolCallID: message.toolCallID})
			continue
		}
		content := make([]map[string]any, 0, len(message.attachments)+1)
		for attachmentIndex := range message.attachments {
			attachment := &message.attachments[attachmentIndex]
			data, dataError := attachment.bytes()
			if dataError != nil {
				return nil, dataError
			}
			content = append(content, map[string]any{
				"type": "image_url",
				"image_url": map[string]string{
					"url": "data:" + attachment.mimeType + ";base64," + base64.StdEncoding.EncodeToString(data),
				},
			})
		}
		content = append(content, map[string]any{"type": "text", "text": message.content})
		providerMessages = append(providerMessages, chatCompletionMessage{Role: string(message.role), Content: content})
	}
	return providerMessages, nil
}

func (messages chatMessages) requestDisplayText() string {
	return messages.responseVisibleMessages().openAIResponsesTextInput()
}

func (messages chatMessages) responseRequestMessages() []map[string]any {
	visibleMessages := messages.responseVisibleMessages()
	responseMessages := make([]map[string]any, 0, len(visibleMessages))
	for _, message := range visibleMessages {
		responseMessage := map[string]any{
			"role":    string(message.role),
			"content": message.content,
		}
		if len(message.toolCalls) > 0 {
			responseMessage["tool_calls"] = message.toolCalls
		}
		if message.toolCallID != "" {
			responseMessage["tool_call_id"] = message.toolCallID
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
