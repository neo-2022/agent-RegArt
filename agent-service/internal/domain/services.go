package domain

type LLMProvider interface {
	Chat(req *LLMRequest) (*LLMResponse, error)
	ListModels() ([]string, error)
	Name() string
}

type LLMRequest struct {
	Model    string
	Messages []LLMMessage
	Tools    []LLMTool
	Stream   bool
}

type LLMMessage struct {
	Role       string
	Content    string
	ToolCalls  []LLMToolCall
	ToolCallID string
}

type LLMToolCall struct {
	ID       string
	Type     string
	Function LLMFunctionCall
}

type LLMFunctionCall struct {
	Name      string
	Arguments string
}

type LLMTool struct {
	Type     string
	Function LLMFunctionDef
}

type LLMFunctionDef struct {
	Name        string
	Description string
	Parameters  interface{}
}

type LLMResponse struct {
	Content   string
	ToolCalls []LLMToolCall
	Model     string
}

type ToolExecutor interface {
	Execute(toolName string, args map[string]interface{}) (map[string]interface{}, error)
}

type RAGRetriever interface {
	Search(query string, topK int) ([]string, error)
	AddDocument(title, content, source string) error
}
