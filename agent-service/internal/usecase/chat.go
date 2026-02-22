// Package usecase — слой бизнес-логики (use cases) agent-service.
//
// Содержит сценарии использования, которые оркестрируют доменные сервисы:
// работу с чатами, агентами, RAG-поиском и вызовом инструментов.
package usecase

import "github.com/neo-2022/openclaw-memory/agent-service/internal/domain"

// ChatUseCase — сценарий работы с чатом.
// Оркестрирует взаимодействие между агентом, сообщениями, RAG и инструментами.
type ChatUseCase struct {
	agents   domain.AgentRepository   // Репозиторий агентов
	messages domain.MessageRepository // Репозиторий сообщений
	rag      domain.RAGRetriever      // RAG-поиск по базе знаний
	tools    domain.ToolExecutor      // Исполнитель инструментов
}

// Создаёт новый экземпляр ChatUseCase с заданными зависимостями.
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

// Получить агента по имени.
func (uc *ChatUseCase) GetAgent(name string) (*domain.Agent, error) {
	return uc.agents.GetByName(name)
}

// Сохранить сообщение в репозитории.
func (uc *ChatUseCase) SaveMessage(msg *domain.Message) error {
	return uc.messages.Save(msg)
}

// Выполнить семантический поиск по базе знаний.
// Возвращает topK наиболее релевантных фрагментов. Если RAG не настроен, возвращает nil.
func (uc *ChatUseCase) SearchRAG(query string, topK int) ([]string, error) {
	if uc.rag == nil {
		return nil, nil
	}
	return uc.rag.Search(query, topK)
}

// Вызвать инструмент по имени с заданными аргументами.
// Делегирует выполнение в tools-service через ToolExecutor.
func (uc *ChatUseCase) ExecuteTool(name string, args map[string]interface{}) (map[string]interface{}, error) {
	return uc.tools.Execute(name, args)
}
