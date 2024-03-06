package gemini

type Response struct {
	Candidates     []Candidate    `json:"candidates"`
	PromptFeedback PromptFeedback `json:"promptFeedback"`
	// UsageMetadata UsageMetadata `json:"usageMetadata"`
}

// Candidate is the element of Response
type Candidate struct {
	Content      *CandidateContent `json:"content"`
	FinishReason string            `json:"finishReason"`
	Index        int               `json:"index"`
	// SafetyRatings []CandidateSafetyRating `json:"safetyRatings"`
}

// CandidateContent is the content of Candidate
type CandidateContent struct {
	Parts []*Part `json:"parts"`
	Role  string  `json:"role"`
}

// Part is the element of CandidateContent
type Part struct {
	FunctionCall *FunctionCall `json:"functionCall"`
}

// FunctionCall is the functionCall of Part
type FunctionCall struct {
	Name string                 `json:"name"`
	Args map[string]interface{} `json:"args"`
}

// CandidateSafetyRating is the safetyRatings of Candidate
type CandidateSafetyRating struct {
	Category    string `json:"category"`
	Probability string `json:"probability"`
}

// UsageMetadata is the token usage in Response
type UsageMetadata struct {
	PromptTokenCount int `json:"promptTokenCount"`
	TotalTokenCount  int `json:"totalTokenCount"`
}

// SafetyRating is the element of PromptFeedback
type SafetyRating struct {
	Category    string `json:"category"`
	Probability string `json:"probability"`
}

// PromptFeedback is the feedback of Prompt
type PromptFeedback struct {
	SafetyRatings []*SafetyRating `json:"safetyRatings"`
}
