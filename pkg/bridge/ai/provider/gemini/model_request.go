package gemini

// RequestBody is the request body
type RequestBody struct {
	Contents Contents `json:"contents"`
	Tools    []Tool   `json:"tools"`
}

// Contents is the contents in RequestBody
type Contents struct {
	Role  string `json:"role"`
	Parts Parts  `json:"parts"`
}

// Parts is the contents.parts in RequestBody
type Parts struct {
	Text string `json:"text"`
}

// Tool is the element of tools in RequestBody
type Tool struct {
	FunctionDeclarations []*FunctionDeclaration `json:"function_declarations"`
}

// FunctionDeclaration is the element of Tool
type FunctionDeclaration struct {
	Name        string              `json:"name"`
	Description string              `json:"description"`
	Parameters  *FunctionParameters `json:"parameters"`
}

// FunctionParameters is the parameters of FunctionDeclaration
type FunctionParameters struct {
	Type       string               `json:"type"`
	Properties map[string]*Property `json:"properties"`
	Required   []string             `json:"required"`
}

// Property is the element of ParameterProperties
type Property struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}
