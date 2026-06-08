package proxy

import (
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

type chatMessage struct {
	role              chatRole
	content           string
	order             *int
	visibleInResponse bool
}

type chatMessages []chatMessage

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
	if len(payloadMessages) == 0 {
		return nil, fmt.Errorf("%w: empty messages", ErrInvalidChatMessages)
	}
	orderedPayloadMessages, orderError := sortPayloadMessagesByOrder(payloadMessages)
	if orderError != nil {
		return nil, orderError
	}
	messages := chatMessages{}
	hasSystemMessage := false
	hasUserMessage := false
	for messageIndex, payloadMessage := range orderedPayloadMessages {
		role, roleError := newChatRole(payloadMessage.Role)
		if roleError != nil {
			return nil, fmt.Errorf("%w: messages[%d].role", roleError, messageIndex)
		}
		if utils.IsBlank(payloadMessage.Content) {
			return nil, fmt.Errorf("%w: messages[%d].content is empty", ErrInvalidChatMessages, messageIndex)
		}
		if role == chatRoleSystem {
			hasSystemMessage = true
		}
		if role == chatRoleUser {
			hasUserMessage = true
		}
		messages = append(messages, chatMessage{role: role, content: payloadMessage.Content, order: payloadMessage.Order, visibleInResponse: true})
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

func sortPayloadMessagesByOrder(payloadMessages []chatMessagePayload) ([]chatMessagePayload, error) {
	orderedPayloadMessages := append([]chatMessagePayload(nil), payloadMessages...)
	hasExplicitOrder := false
	for _, payloadMessage := range orderedPayloadMessages {
		if payloadMessage.Order != nil {
			hasExplicitOrder = true
			break
		}
	}
	if !hasExplicitOrder {
		return orderedPayloadMessages, nil
	}
	seenOrders := map[int]struct{}{}
	for messageIndex, payloadMessage := range orderedPayloadMessages {
		if payloadMessage.Order == nil {
			return nil, fmt.Errorf("%w: messages[%d].order missing", ErrInvalidChatMessages, messageIndex)
		}
		if *payloadMessage.Order < 0 {
			return nil, fmt.Errorf("%w: messages[%d].order is negative", ErrInvalidChatMessages, messageIndex)
		}
		if _, exists := seenOrders[*payloadMessage.Order]; exists {
			return nil, fmt.Errorf("%w: duplicate messages[].order=%d", ErrInvalidChatMessages, *payloadMessage.Order)
		}
		seenOrders[*payloadMessage.Order] = struct{}{}
	}
	sort.SliceStable(orderedPayloadMessages, func(firstIndex int, secondIndex int) bool {
		return *orderedPayloadMessages[firstIndex].Order < *orderedPayloadMessages[secondIndex].Order
	})
	return orderedPayloadMessages, nil
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
