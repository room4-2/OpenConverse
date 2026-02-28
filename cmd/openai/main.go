package main

import (
	"fmt"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
)

func main(){
	tool := []responses.ToolUnionParam{{
		OfFunction: &responses.FunctionToolParam{
			Name:        "get_weather",
			Description: openai.String("Get weather at the given location"),
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"location": map[string]string{
						"type": "string",
					},
				},
				"required": []string{"location"},
			},
		},
	}}
	mytool := tool[0]
	
	bytesMarshal,err := mytool.MarshalJSON()
		fmt.Println(string(bytesMarshal),err)

}