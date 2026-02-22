package domain

// LLMProvider — интерфейс провайдера языковых моделей (LLM).
// Реализации этого интерфейса обеспечивают взаимодействие с конкретными LLM
// (Ollama, OpenAI, YandexGPT, Anthropic и т.д.).
type LLMProvider interface {
	// Отправить запрос к LLM и получить ответ (с поддержкой вызова инструментов).
	Chat(req *LLMRequest) (*LLMResponse, error)
	// Получить список доступных моделей у провайдера.
	ListModels() ([]string, error)
	// Вернуть имя провайдера (ollama, openai, yandexgpt и т.д.).
	Name() string
}

// LLMRequest — запрос к языковой модели.
// Содержит модель, историю сообщений, доступные инструменты и флаг стриминга.
type LLMRequest struct {
	Model    string       // Имя модели (например, llama3.1:8b)
	Messages []LLMMessage // История сообщений (контекст диалога)
	Tools    []LLMTool    // Список доступных инструментов для вызова
	Stream   bool         // Флаг потоковой передачи ответа (SSE)
}

// LLMMessage — сообщение в диалоге с LLM.
// Используется для формирования контекста запроса (системный промпт, сообщения пользователя,
// ответы ассистента, результаты вызовов инструментов).
type LLMMessage struct {
	Role       string        // Роль: system, user, assistant, tool
	Content    string        // Текст сообщения
	ToolCalls  []LLMToolCall // Вызовы инструментов (заполняется в ответе ассистента)
	ToolCallID string        // ID вызова инструмента (для сообщений с role=tool)
}

// LLMToolCall — вызов инструмента, запрошенный моделью.
// Модель возвращает этот объект, когда решает, что нужно вызвать инструмент.
type LLMToolCall struct {
	ID       string          // Уникальный ID вызова
	Type     string          // Тип (всегда "function")
	Function LLMFunctionCall // Описание вызываемой функции
}

// LLMFunctionCall — конкретный вызов функции с именем и аргументами.
type LLMFunctionCall struct {
	Name      string // Имя функции (например, execute_command, read_file)
	Arguments string // JSON-строка с аргументами вызова
}

// LLMTool — описание инструмента, доступного модели.
// Передаётся в запросе к LLM для включения режима tool calling.
type LLMTool struct {
	Type     string         // Тип (всегда "function")
	Function LLMFunctionDef // Определение функции
}

// LLMFunctionDef — определение функции-инструмента.
// Содержит имя, описание и JSON Schema параметров.
type LLMFunctionDef struct {
	Name        string      // Имя функции
	Description string      // Описание функции (для LLM)
	Parameters  interface{} // JSON Schema параметров функции
}

// LLMResponse — ответ от языковой модели.
// Содержит текстовый ответ и/или запросы на вызов инструментов.
type LLMResponse struct {
	Content   string        // Текстовый ответ модели
	ToolCalls []LLMToolCall // Запрошенные вызовы инструментов (может быть пуст)
	Model     string        // Имя модели, сгенерировавшей ответ
}

// ToolExecutor — интерфейс исполнителя инструментов.
// Выполняет команды, операции с файлами и другие действия по запросу агента.
type ToolExecutor interface {
	// Выполнить инструмент по имени с заданными аргументами.
	// Возвращает результат в виде map (ключ-значение) или ошибку.
	Execute(toolName string, args map[string]interface{}) (map[string]interface{}, error)
}

// RAGRetriever — интерфейс для работы с RAG (Retrieval-Augmented Generation).
// Обеспечивает семантический поиск по базе знаний и добавление новых документов.
type RAGRetriever interface {
	// Найти topK наиболее релевантных документов по запросу.
	Search(query string, topK int) ([]string, error)
	// Добавить документ в базу знаний (заголовок, содержимое, источник).
	AddDocument(title, content, source string) error
}
