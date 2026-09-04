package proxy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func openAIUsage(usage *tokenUsage, protocol clientTextProtocol) any {
	if usage == nil {
		return nil
	}
	if protocol == clientChatProtocol {
		return gin.H{"prompt_tokens": usage.RequestTokens, "completion_tokens": usage.ResponseTokens, "total_tokens": usage.TotalTokens}
	}
	return gin.H{"input_tokens": usage.RequestTokens, "output_tokens": usage.ResponseTokens, "total_tokens": usage.TotalTokens}
}
func encodeOpenAICompletion(c *gin.Context, protocol clientTextProtocol, input decodedClientCompletion, request chatRequestParameters, result completionResult) {
	created := time.Now().Unix()
	id := requestIDFromContext(c)
	model := request.provider.identifier.string() + "/" + request.model.string()
	if protocol == clientChatProtocol {
		finish := "stop"
		message := gin.H{"role": "assistant", "content": result.content.text(), "refusal": nil}
		if len(result.content.toolCalls()) > 0 {
			finish = "tool_calls"
			message["tool_calls"] = chatFunctionCalls(result.content.toolCalls())
			if result.content.text() == "" {
				message["content"] = nil
			}
		}
		if !input.stream {
			c.JSON(http.StatusOK, gin.H{"id": "chatcmpl-" + id, "object": "chat.completion", "created": created, "model": model, "choices": []any{gin.H{"index": 0, "message": message, "finish_reason": finish, "logprobs": nil}}, "usage": openAIUsage(result.usage, protocol)})
			return
		}
		stream := newClientEventStream(c)
		chunk := func(delta gin.H, reason any) gin.H {
			return gin.H{"id": "chatcmpl-" + id, "object": "chat.completion.chunk", "created": created, "model": model, "choices": []any{gin.H{"index": 0, "delta": delta, "finish_reason": reason, "logprobs": nil}}, "usage": nil}
		}
		stream.data(chunk(gin.H{"role": "assistant", "content": ""}, nil))
		if result.content.text() != "" {
			stream.data(chunk(gin.H{"content": result.content.text()}, nil))
		}
		for index, call := range chatFunctionCalls(result.content.toolCalls()) {
			stream.data(chunk(gin.H{"tool_calls": []any{gin.H{"index": index, "id": call.ID, "type": "function", "function": call.Function}}}, nil))
		}
		stream.data(chunk(gin.H{}, finish))
		if input.includeUsage {
			stream.data(gin.H{"id": "chatcmpl-" + id, "object": "chat.completion.chunk", "created": created, "model": model, "choices": []any{}, "usage": openAIUsage(result.usage, protocol)})
		}
		stream.done()
		return
	}
	items := make([]gin.H, 0, len(result.content.toolCalls())+1)
	if result.content.text() != "" {
		items = append(items, gin.H{"type": "message", "id": "msg_" + id, "status": "completed", "role": "assistant", "content": []any{gin.H{"type": "output_text", "text": result.content.text(), "annotations": []any{}, "logprobs": []any{}}}})
	}
	for index, call := range result.content.toolCalls() {
		items = append(items, gin.H{"type": "function_call", "id": fmt.Sprintf("fc_%s_%d", id, index), "call_id": call.ID, "name": call.Name, "arguments": call.Arguments, "status": "completed"})
	}
	response := gin.H{"id": "resp_" + id, "object": "response", "created_at": created, "completed_at": created, "status": "completed", "model": model, "output": items, "error": nil, "incomplete_details": nil, "instructions": input.instructions, "temperature": nil, "top_p": nil, "max_output_tokens": input.maxTokens, "parallel_tool_calls": true, "previous_response_id": nil, "store": false, "tools": []any{}, "tool_choice": "auto", "truncation": "disabled", "metadata": gin.H{}, "usage": openAIUsage(result.usage, protocol)}
	if request.tools != nil {
		response["tools"] = request.tools.responsesTools()
		response["tool_choice"] = request.tools.responsesChoice()
		if request.tools.parallel != nil {
			response["parallel_tool_calls"] = *request.tools.parallel
		}
	}
	if !input.stream {
		c.JSON(http.StatusOK, response)
		return
	}
	stream := newClientEventStream(c)
	pending := gin.H{}
	for key, value := range response {
		pending[key] = value
	}
	pending["status"] = "in_progress"
	pending["output"] = []any{}
	pending["usage"] = nil
	pending["completed_at"] = nil
	stream.event("response.created", gin.H{"response": pending})
	stream.event("response.in_progress", gin.H{"response": pending})
	for index, item := range items {
		initial := gin.H{}
		for key, value := range item {
			initial[key] = value
		}
		initial["status"] = "in_progress"
		if item["type"] == "message" {
			initial["content"] = []any{}
		} else {
			initial["arguments"] = ""
		}
		stream.event("response.output_item.added", gin.H{"output_index": index, "item": initial})
		if item["type"] == "message" {
			emptyPart := gin.H{"type": "output_text", "text": "", "annotations": []any{}, "logprobs": []any{}}
			stream.event("response.content_part.added", gin.H{"item_id": item["id"], "output_index": index, "content_index": 0, "part": emptyPart})
			stream.event("response.output_text.delta", gin.H{"item_id": item["id"], "output_index": index, "content_index": 0, "delta": result.content.text(), "logprobs": []any{}})
			stream.event("response.output_text.done", gin.H{"item_id": item["id"], "output_index": index, "content_index": 0, "text": result.content.text(), "logprobs": []any{}})
			stream.event("response.content_part.done", gin.H{"item_id": item["id"], "output_index": index, "content_index": 0, "part": item["content"].([]any)[0]})
		} else {
			stream.event("response.function_call_arguments.delta", gin.H{"item_id": item["id"], "output_index": index, "delta": item["arguments"]})
			stream.event("response.function_call_arguments.done", gin.H{"item_id": item["id"], "output_index": index, "arguments": item["arguments"], "name": item["name"]})
		}
		stream.event("response.output_item.done", gin.H{"output_index": index, "item": item})
	}
	stream.event("response.completed", gin.H{"response": response})
	stream.send()
}

// The event sequence is buffered because the coordinator returns a final result.
type clientEventStream struct {
	context  *gin.Context
	sequence int
	buffer   strings.Builder
}

func newClientEventStream(c *gin.Context) *clientEventStream { return &clientEventStream{context: c} }
func (stream *clientEventStream) data(value any) {
	encoded, _ := json.Marshal(value)
	stream.buffer.WriteString("data: " + string(encoded) + "\n\n")
}
func (stream *clientEventStream) event(kind string, value gin.H) {
	value["type"] = kind
	value["sequence_number"] = stream.sequence
	stream.sequence++
	encoded, _ := json.Marshal(value)
	stream.buffer.WriteString("event: " + kind + "\ndata: " + string(encoded) + "\n\n")
}
func (stream *clientEventStream) done() { stream.buffer.WriteString("data: [DONE]\n\n"); stream.send() }
func (stream *clientEventStream) send() {
	stream.context.Header("Cache-Control", "no-store")
	stream.context.Header("X-Accel-Buffering", "no")
	stream.context.Data(http.StatusOK, "text/event-stream", []byte(stream.buffer.String()))
}
