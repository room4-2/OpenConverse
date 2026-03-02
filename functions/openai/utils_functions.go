package openai

import (
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
)

// GetHangUpFunction returns the function tool parameter for hanging up a call.
func GetHangUpFunction() *responses.FunctionToolParam {
	return &responses.FunctionToolParam{
		Name:        "hangUp",
		Description: openai.String("This function is a way for the agent to stop the call"),
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"reason": map[string]string{
					"type": "string",
				},
			},
			"required": []string{"reason"},
		},
	}
}
