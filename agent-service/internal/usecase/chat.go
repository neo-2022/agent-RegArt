package usecase

import "github.com/neo-2022/openclaw-memory/agent-service/internal/domain"

type ChatUseCase struct {
	agents   domain.AgentRepository
	messages domain.MessageRepository
	rag      domain.RAGRetriever
	tools    domain.ToolExecutor
}

func NewChatUseCase(
	agents domain.AgentRepository,
	messages domain.MessageRepository,
	rag domain.RAGRetriever,
	tools domain.ToolExecutor,
) *ChatUseCase {
	return &ChatUseCase{
		agents:   agents,
		messages: messages,
		rag:      rag,
		tools:    tools,
	}
}

func (uc *ChatUseCase) GetAgent(name string) (*domain.Agent, error) {
	return uc.agents.GetByName(name)
}

func (uc *ChatUseCase) SaveMessage(msg *domain.Message) error {
	return uc.messages.Save(msg)
}

func (uc *ChatUseCase) SearchRAG(query string, topK int) ([]string, error) {
	if uc.rag == nil {
		return nil, nil
	}
	return uc.rag.Search(query, topK)
}

func (uc *ChatUseCase) ExecuteTool(name string, args map[string]interface{}) (map[string]interface{}, error) {
	return uc.tools.Execute(name, args)
}
