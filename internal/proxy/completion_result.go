package proxy

// completionContent is the closed set of successful canonical results.
// Provider attempts stay separate from this client-facing domain result.
type completionContent interface {
	text() string
	toolCalls() []functionCall
}
type completedText string

func (value completedText) text() string        { return string(value) }
func (completedText) toolCalls() []functionCall { return nil }

type completedStructuredData string

func (value completedStructuredData) text() string        { return string(value) }
func (completedStructuredData) toolCalls() []functionCall { return nil }

type completedFunctionCalls struct {
	visibleText string
	calls       []functionCall
}

func (value completedFunctionCalls) text() string              { return value.visibleText }
func (value completedFunctionCalls) toolCalls() []functionCall { return value.calls }

type completionResult struct {
	content completionContent
	usage   *tokenUsage
}
