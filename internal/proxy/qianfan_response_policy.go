package proxy

import (
	"encoding/json"
	"fmt"
)

type chatCompletionResponsePolicy string

const (
	chatCompletionResponsePolicyDefault chatCompletionResponsePolicy = ""
	chatCompletionResponsePolicyQianfan chatCompletionResponsePolicy = "qianfan"
)

func validateQianfanChatChoices(choices []chatCompletionChoice) error {
	for _, choice := range choices {
		if choice.FinishReason != finishReasonStop && choice.FinishReason != "length" {
			return fmt.Errorf("%w: Qianfan response has an unsupported finish reason", ErrProviderAPI)
		}
		if len(choice.Flag) == 0 {
			continue
		}
		var flag *int
		if err := json.Unmarshal(choice.Flag, &flag); err != nil || flag == nil || (*flag != 0 && *flag != 1) {
			return fmt.Errorf("%w: Qianfan response has a blocked or invalid flag", ErrProviderAPI)
		}
	}
	return nil
}
