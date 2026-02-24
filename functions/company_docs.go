package functions

import "google.golang.org/genai"

// GetNaboopayInformationsDocsFunctionDeclaration returns the function declaration for Gemini
func GetCompanyInformationsDocsFunctionDeclaration() *genai.FunctionDeclaration {
	return &genai.FunctionDeclaration{
		Name:        "GetCompanyInformationsDocs",
		Description: "Get All the information of the company"}
}

var docs string = `
my company name is NabooPay, we are specialized in payment processing
for e-commerce websites and online services. We offer competitive rates, 
secure transactions, and seamless integration with popular platforms.
`

func GetCompanyInformationsDocs() string {
	return docs
}
