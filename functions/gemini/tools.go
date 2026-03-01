package gemini

import "google.golang.org/genai"

// GetHangUpFunctionDeclaration returns the Gemini function declaration for hanging up a call.
func GetHangUpFunctionDeclaration() *genai.FunctionDeclaration {
	return &genai.FunctionDeclaration{
		Name:        "hangUp",
		Description: "This function is a way for the agent to stop the call",
		Parameters: &genai.Schema{
			Type: genai.TypeObject,
			Properties: map[string]*genai.Schema{
				"reason": {Type: genai.TypeString},
			},
			Required: []string{"reason"},
		},
	}
}
