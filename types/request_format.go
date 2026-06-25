package types

// RequestFormat identifies the client-facing request shape before relay conversion.
type RequestFormat string

const (
	RequestFormatUnknown   RequestFormat = "unknown"
	RequestFormatClaude    RequestFormat = "claude"
	RequestFormatOpenAI    RequestFormat = "openai"
	RequestFormatResponses RequestFormat = "response"
	RequestFormatGemini    RequestFormat = "gemini"
	RequestFormatGeminiCLI RequestFormat = "gemini_cli"
)
