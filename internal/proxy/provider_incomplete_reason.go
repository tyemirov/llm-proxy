package proxy

import "fmt"

const (
	providerIncompleteOutputLimitReason  = "max_output_tokens"
	maximumProviderIncompleteReasonBytes = 128
)

// newProviderIncompleteReason accepts provider reason codes, never free-form text.
func newProviderIncompleteReason(value string) (string, error) {
	if len(value) == 0 || len(value) > maximumProviderIncompleteReasonBytes {
		return "", fmt.Errorf("%w: invalid incomplete reason code", ErrProviderAPI)
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') || character == '_' || character == '-' || character == '.' || character == ':' {
			continue
		}
		return "", fmt.Errorf("%w: invalid incomplete reason code", ErrProviderAPI)
	}
	return value, nil
}
