// Пакет main — точка входа HTTP-сервера agent-service.
// Это центральный микросервис системы Agent Core NG, который отвечает за:
//   - Обработку чат-запросов от пользователя через /chat
//   - Управление агентами (admin, coder, novice) через /agents
//   - Работу с локальными (Ollama) и облачными (OpenAI, Anthropic, YandexGPT, GigaChat) LLM
//   - Оркестрацию: Admin может вызывать Coder и Novice как инструменты (tool calling)
//   - Сохранение истории чатов в PostgreSQL
//   - Управление промптами, аватарами, моделями, провайдерами и рабочими пространствами
//
// HTTP-эндпоинты:
//   - /health            — проверка состояния сервиса
//   - /chat              — основной чат с агентами (POST)
//   - /agents            — список агентов с их настройками (GET)
//   - /models            — список локальных моделей Ollama с поддержкой инструментов (GET)
//   - /prompts           — список файлов промптов для агента (GET)
//   - /prompts/load      — загрузка промпта из файла (POST)
//   - /agent/prompt      — обновление промпта вручную (POST)
//   - /update-model      — смена модели и провайдера для агента (POST)
//   - /avatar            — загрузка аватара агента (POST)
//   - /avatar-info       — получение информации об аватаре (GET)
//   - /providers         — управление облачными LLM-провайдерами (GET/POST)
//   - /cloud-models      — список моделей облачного провайдера (GET)
//   - /workspaces        — управление рабочими пространствами (GET/POST/DELETE)
//   - /uploads/          — раздача статических файлов (аватары и др.)
//
// Порт по умолчанию: 8083 (настраивается через AGENT_SERVICE_PORT).
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/neo-2022/openclaw-memory/agent-service/internal/metrics"
	"github.com/neo-2022/openclaw-memory/agent-service/internal/rag"

	"github.com/neo-2022/openclaw-memory/agent-service/internal/db"
	"github.com/neo-2022/openclaw-memory/agent-service/internal/handlers"
	"github.com/neo-2022/openclaw-memory/agent-service/internal/intent"
	"github.com/neo-2022/openclaw-memory/agent-service/internal/llm"
	"github.com/neo-2022/openclaw-memory/agent-service/internal/models"
	"github.com/neo-2022/openclaw-memory/agent-service/internal/repository"
	"github.com/neo-2022/openclaw-memory/agent-service/internal/tools"
)

// ChatRequest — структура входящего запроса на /chat.
// Содержит историю сообщений и имя агента, которому адресован запрос.
//
// Поля:
//   - Messages: массив сообщений (история диалога), включая роли user, assistant, system
//   - Agent: имя агента (admin, coder, novice), который должен ответить
type ChatRequest struct {
	Messages []llm.Message `json:"messages"`
	Agent    string        `json:"agent"`
}

// ChatResponse — структура ответа от /chat.
// Содержит текст ответа агента или сообщение об ошибке.
//
// Поля:
//   - Response: текст ответа от LLM (через выбранного провайдера)
//   - Error: сообщение об ошибке (опционально, omitempty — не включается если пусто)
//   - Sources: источники RAG (опционально, для отображения в UI)
type ChatResponse struct {
	Response string   `json:"response"`
	Error    string   `json:"error,omitempty"`
	Sources  []Source `json:"sources,omitempty"`
}

// Source представляет источник RAG для отображения в UI
type Source struct {
	Title   string `json:"title"`
	Content string `json:"content"`
	Score   int    `json:"score"`
}

// UpdateModelRequest — структура запроса на смену модели агента (POST /update-model).
// Позволяет изменить LLM-модель и/или провайдера для конкретного агента.
//
// Поля:
//   - Agent: имя агента (admin, coder, novice)
//   - Model: название модели (например, "gpt-4o", "llama3.1:8b", "yandexgpt")
//   - Provider: имя провайдера (ollama, openai, anthropic, yandexgpt, gigachat)
type UpdateModelRequest struct {
	Agent    string `json:"agent"`
	Model    string `json:"model"`
	Provider string `json:"provider"`
}

// UpdatePromptRequest — структура запроса на обновление системного промпта (POST /agent/prompt).
// Используется при ручном редактировании промпта через UI (кнопка-карандаш).
//
// Поля:
//   - Agent: имя агента
//   - Prompt: новый текст системного промпта
type UpdatePromptRequest struct {
	Agent  string `json:"agent"`
	Prompt string `json:"prompt"`
}

// getEnv — вспомогательная функция для чтения переменных окружения.
// Если переменная не задана или пуста — возвращает значение по умолчанию.
//
// Параметры:
//   - key: имя переменной окружения
//   - defaultValue: значение по умолчанию
//
// Возвращает: значение переменной окружения или defaultValue.
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// Глобальный RAG-ретривер для поиска документов
var ragRetriever *rag.DBRetriever

// initRAG — инициализация RAG-системы при старте.
// Загружает конфигурацию из переменных окружения и создаёт экземпляр DBRetriever.
func initRAG() {
	chromaURL := getEnv("CHROMA_URL", "")
	embModel := getEnv("EMBEDDING_MODEL", "nomic-embed-text")
	topK := 5
	if k := getEnv("RAG_TOP_K", ""); k != "" {
		if parsed, err := strconv.Atoi(k); err == nil {
			topK = parsed
		}
	}

	maxChunkLen := rag.DefaultMaxChunkLen
	if v := getEnv("RAG_MAX_CHUNK_LEN", ""); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			maxChunkLen = parsed
		}
	}

	maxContextLen := rag.DefaultMaxContextLen
	if v := getEnv("RAG_MAX_CONTEXT_LEN", ""); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			maxContextLen = parsed
		}
	}

	config := &rag.Config{
		ChromaURL:      chromaURL,
		EmbeddingModel: embModel,
		TopK:           topK,
		MaxChunkLen:    maxChunkLen,
		MaxContextLen:  maxContextLen,
		DBHost:         getEnv("DB_HOST", "localhost"),
		DBPort:         getEnv("DB_PORT", "5432"),
		DBUser:         getEnv("DB_USER", "postgres"),
		DBPassword:     getEnv("DB_PASSWORD", "postgres"),
		DBName:         getEnv("DB_NAME", "agentcore"),
	}

	ragRetriever = rag.NewDBRetriever(config)
	log.Printf("[RAG] Инициализирован: ChromA=%s, модель=%s, topK=%d", chromaURL, embModel, topK)

	// Загружаем демо-документы в ChromA при первом запуске
	if chromaURL != "" {
		go func() {
			time.Sleep(2 * time.Second) // Даём время ChromA запуститься
			if err := ragRetriever.SeedDemoDocuments(); err != nil {
				log.Printf("[RAG] Ошибка загрузки демо-документов: %v", err)
			}
		}()
	}
}

// resolveToolRoute — определяет базовый URL сервиса и путь эндпоинта для инструмента.
// Инструменты tools-service (порт 8082): execute, read, write, list, delete, sysinfo и т.д.
// Инструменты browser-service (порт 8084): browser_*, input_*, internet_search, crawler_*, check_url_access и т.д.
// Возвращает (baseURL, path). Если инструмент не найден — возвращает tools-service с /toolName.
func resolveToolRoute(toolName string) (string, string) {
	toolsURL := getEnv("TOOLS_SERVICE_URL", "http://localhost:8082")
	browserURL := getEnv("BROWSER_SERVICE_URL", "http://localhost:8084")

	// Маппинг имён инструментов → (сервис, эндпоинт)
	browserRoutes := map[string]string{
		"browser_get_dom":        "/browser/dom",
		"browser_open_visible":   "/browser/open",
		"browser_screenshot":     "/browser/screenshot",
		"browser_pdf":            "/browser/pdf",
		"browser_get_text":       "/browser/text",
		"browser_get_title":      "/browser/title",
		"browser_execute_js":     "/browser/js",
		"browser_detect_captcha": "/browser/captcha",
		"input_key_press":        "/input/key",
		"input_type_text":        "/input/type",
		"input_mouse_click":      "/input/click",
		"input_mouse_move":       "/input/move",
		"input_mouse_scroll":     "/input/scroll",
		"input_mouse_drag":       "/input/drag",
		"input_tab_action":       "/input/tab",
		"input_window_action":    "/input/window",
		"input_clipboard":        "/input/clipboard",
		"internet_search":        "/search",
		"crawler_fetch":          "/crawler/fetch",
		"crawler_robots_txt":     "/crawler/robots",
		"check_url_access":       "/access/check",
		"check_multiple_urls":    "/access/check-multiple",
	}

	if path, ok := browserRoutes[toolName]; ok {
		return browserURL, path
	}

	// Всё остальное — tools-service (execute, read, write, list, delete, sysinfo, sysload, cputemp и т.д.)
	return toolsURL, "/" + toolName
}

// callTool — вызов инструмента через tools-service или browser-service по HTTP.
// Автоматически маршрутизирует запрос к нужному микросервису по имени инструмента.
// Обрабатывает JSON и не-JSON ответы (404, ошибки сервера и т.д.).
//
// Параметры:
//   - toolName: имя инструмента (execute, read, write, sysinfo, browser_get_dom и др.)
//   - args: аргументы инструмента в виде map (будут сериализованы в JSON)
//
// Возвращает:
//   - map[string]interface{}: результат выполнения инструмента
//   - error: ошибка HTTP-запроса
func callTool(toolName string, args map[string]interface{}) (map[string]interface{}, error) {
	baseURL, path := resolveToolRoute(toolName)
	fullURL := baseURL + path
	log.Printf("callTool: %s → %s (args: %+v)", toolName, fullURL, args)

	data, err := json.Marshal(args)
	if err != nil {
		return nil, err
	}
	resp, err := http.Post(fullURL, "application/json", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Читаем тело ответа целиком для надёжного парсинга
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения ответа: %v", err)
	}

	// Если HTTP-статус не 2xx — возвращаем ошибку с телом ответа
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return map[string]interface{}{
			"error":       fmt.Sprintf("HTTP %d от %s", resp.StatusCode, fullURL),
			"status_code": resp.StatusCode,
			"body":        string(bodyBytes),
		}, nil
	}

	// Попытка 1: JSON-объект
	var result map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &result); err == nil {
		return result, nil
	}

	// Попытка 2: JSON-массив
	var arrResult []interface{}
	if err := json.Unmarshal(bodyBytes, &arrResult); err == nil {
		return map[string]interface{}{"result": arrResult}, nil
	}

	// Попытка 3: JSON-строка или число
	var anyResult interface{}
	if err := json.Unmarshal(bodyBytes, &anyResult); err == nil {
		return map[string]interface{}{"result": anyResult}, nil
	}

	// Fallback: просто текст
	return map[string]interface{}{"result": string(bodyBytes)}, nil
}

// chatHandler— основной обработчик чат-запросов (POST /chat).
// Это главная точка взаимодействия пользователя с AI-агентами.
//
// Алгоритм обработки запроса:
//  1. Валидация запроса (метод POST, корректный JSON, непустые сообщения)
//  2. Определение intent (намерения) из последнего сообщения пользователя.
//     Если обнаружен специальный intent (например, запрос поиска приложения),
//     обрабатывается через intent-хэндлер без вызова LLM.
//  3. Загрузка агента из БД и определение провайдера (ollama по умолчанию)
//  4. Формирование сообщений: системный промпт + история диалога
//  5. Отправка запроса к LLM через выбранного провайдера
//  6. Обработка tool calls (вызовов инструментов):
//     a) Стандартные tool calls (формат OpenAI): обработка через chatResp.ToolCalls
//     b) JSON tool calls (для моделей без native tool calling): парсинг JSON из ответа
//     Для обоих случаев:
//     - call_coder/call_novice — делегирование другим агентам (только для Admin)
//     - остальные инструменты — вызов через tools-service
//     После выполнения инструментов — повторный запрос к LLM с результатами
//  7. Сохранение сообщений в PostgreSQL (пользовательское + ответ агента)
//  8. Возврат ответа клиенту в формате ChatResponse
//
// chatWithRetry — обёртка над provider.Chat с повторными попытками при транзиентных ошибках (503, 504).
// Бесплатные модели на Routeway/OpenRouter часто возвращают временные ошибки.
// Делаем до 3 попыток с паузой 3 секунды между ними.
func chatWithRetry(provider llm.ChatProvider, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	const maxRetries = 3
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		resp, err := provider.Chat(req)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		errStr := err.Error()
		if strings.Contains(errStr, "429") {
			delay := time.Duration(3*(attempt+1)) * time.Second
			log.Printf("Rate limit 429 (попытка %d/%d): %v. Повтор через %v...", attempt+1, maxRetries, err, delay)
			time.Sleep(delay)
			continue
		}
		if strings.Contains(errStr, "503") || strings.Contains(errStr, "504") || strings.Contains(errStr, "502") {
			delay := 3 * time.Second
			log.Printf("Транзиентная ошибка LLM (попытка %d/%d): %v. Повтор через %v...", attempt+1, maxRetries, err, delay)
			time.Sleep(delay)
			continue
		}
		return nil, err
	}
	return nil, lastErr
}

func chatHandler(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	statusCode := 200

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	if len(req.Messages) == 0 {
		http.Error(w, "Empty messages", http.StatusBadRequest)
		return
	}

	lastMsg := req.Messages[len(req.Messages)-1].Content
	intentType, params := intent.DetectIntent(lastMsg)
	if intentType != intent.IntentNone {
		resp, err := handlers.HandleIntent(intentType, params)
		if err != nil {
			log.Printf("Intent handler error: %v", err)
			writeJSON(w, ChatResponse{Error: err.Error()})
			return
		}
		writeJSON(w, ChatResponse{Response: resp})
		return
	}

	agent, err := repository.GetAgentByName(req.Agent)
	if err != nil {
		log.Printf("Failed to get agent %s: %v", req.Agent, err)
		writeJSON(w, ChatResponse{Error: "Agent not found or no suitable model: " + err.Error()})
		return
	}

	providerName := agent.Provider
	if providerName == "" {
		providerName = "ollama"
	}

	provider, err := llm.GlobalRegistry.Get(providerName)
	if err != nil {
		log.Printf("Provider %s not found: %v", providerName, err)
		WriteSystemLog("error", "agent-service", fmt.Sprintf("Провайдер %s не найден", providerName), err.Error())
		writeJSON(w, ChatResponse{Error: "Provider " + providerName + " not configured"})
		metrics.RecordChatError(req.Agent, providerName, agent.LLMModel, "provider_not_found")
		return
	}

	// Записываем метрику чат-запроса
	metrics.RecordChatRequest(req.Agent, providerName, agent.LLMModel)

	// === RAG: поиск релевантных документов ===
	// Выполняем семантический поиск по базе знаний перед запросом к LLM
	var ragSources []Source
	var ragContext string
	// RAG временно отключен из-за проблем с контекстом
	/*
		log.Printf("[RAG] Выполняю поиск для: %s", truncate(lastMsg, 30))
		ragStartTime := time.Now()
		if ragRetriever != nil {
			results, err := ragRetriever.Search(lastMsg, 5)
			ragDuration := time.Since(ragStartTime)
			if err != nil {
				log.Printf("[RAG] Ошибка поиска: %v", err)
				metrics.RecordRAGSearch("error", 0, ragDuration)
			} else if len(results) > 0 {
				log.Printf("[RAG] Найдено %d документов", len(results))
				metrics.RecordRAGSearch("success", len(results), ragDuration)
				ragContext = "\n\n=== База знаний ===\n"
				for i, r := range results {
					ragContext += fmt.Sprintf("[%d] %s: %s\n", i+1, r.Doc.Title, truncate(r.Doc.Content, 150))
					ragSources = append(ragSources, Source{
						Title:   r.Doc.Title,
						Content: truncate(r.Doc.Content, 100),
						Score:   i + 1,
					})
				}
				ragContext += "Используй эту информацию.\n"
			} else {
				log.Printf("[RAG] Документы не найдены")
				metrics.RecordRAGSearch("empty", 0, ragDuration)
			}
		}
	*/

	// === Система обучения: получение релевантных знаний модели ===
	// Перед каждым запросом к LLM ищем в базе знаний модели
	// релевантные факты и добавляем их в системный промпт.
	systemPrompt := agent.Prompt

	// Временно отключаем learnings из-за проблем с контекстом
	// learnings := fetchModelLearnings(agent.LLMModel, lastMsg)
	var learnings []string
	if len(learnings) > 0 {
		learningContext := "\n\n=== Накопленные знания модели ===\n"
		for i, l := range learnings {
			learningContext += "- " + l + "\n"
			_ = i
		}
		learningContext += "=== Используй эти знания для более точных ответов ===\n"
		systemPrompt += learningContext
		log.Printf("Добавлено %d знаний в контекст модели %s", len(learnings), agent.LLMModel)
	}

	// Добавляем RAG контекст к системному промпту
	if ragContext != "" {
		systemPrompt += ragContext
	}

	messages := make([]llm.Message, 0, len(req.Messages)+1)
	messages = append(messages, llm.Message{Role: "system", Content: systemPrompt})
	messages = append(messages, req.Messages...)

	// LM Studio имеет маленький контекст (4096 токенов) - отключаем инструменты
	supportsTools := agent.SupportsTools && providerName != "lmstudio"

	// Стриминг отключаем когда есть инструменты — Ollama не поддерживает tool calling в режиме stream
	useStream := providerName == "ollama" && !supportsTools
	chatReq := &llm.ChatRequest{
		Model:    agent.LLMModel,
		Messages: messages,
		Stream:   useStream,
	}

	if supportsTools {
		chatReq.Tools = tools.GetToolsForAgent(req.Agent, agent.LLMModel)
		toolNames := make([]string, len(chatReq.Tools))
		for i, t := range chatReq.Tools {
			toolNames[i] = t.Function.Name
		}
		log.Printf("[TOOLS] Агент=%s модель=%s получил %d инструментов: %v", req.Agent, agent.LLMModel, len(chatReq.Tools), toolNames)
	}

	chatResp, err := chatWithRetry(provider, chatReq)
	if err != nil {
		log.Printf("LLM error: %v", err)
		WriteSystemLog("error", "agent-service", fmt.Sprintf("Ошибка LLM (%s/%s): %s", providerName, agent.LLMModel, llm.TranslateLLMError(err.Error())), err.Error())
		writeJSON(w, ChatResponse{Error: llm.TranslateLLMError(err.Error())})
		return
	}

	// === Цикл выполнения инструментов (tool call loop) ===
	// Модели могут вызывать инструменты последовательно: например write→read, или sysinfo→execute.
	// Цикл обрабатывает до 5 раундов tool calls (structured JSON, inline JSON, XML-формат).
	// После каждого вызова результат добавляется в контекст и отправляется повторный запрос к LLM.
	// Цикл завершается когда LLM возвращает обычный текст без tool calls.
	const maxToolRounds = 5
	for round := 0; round < maxToolRounds; round++ {
		log.Printf("Provider %s response (round %d): content=%d chars, tools=%d", providerName, round, len(chatResp.Content), len(chatResp.ToolCalls))

		// --- Вариант 1: Структурированные tool calls (стандартный OpenAI/OpenRouter формат) ---
		if len(chatResp.ToolCalls) > 0 {
			messages = append(messages, llm.Message{Role: "assistant", Content: chatResp.Content, ToolCalls: chatResp.ToolCalls})
			for _, tc := range chatResp.ToolCalls {
				log.Printf("Raw tool call: name=%s, args=%s", tc.Function.Name, string(tc.Function.Arguments))
				args := parseToolArguments(tc.Function.Arguments)
				result := dispatchTool(req.Agent, tc.Function.Name, args, req.Messages)
				log.Printf("Tool %s called with args: %+v, result: %+v", tc.Function.Name, args, result)
				resultBytes, _ := json.Marshal(result)
				messages = append(messages, llm.Message{Role: "tool", Content: string(resultBytes), ToolCallID: tc.ID})
			}
			chatReq.Messages = messages
			chatResp, err = chatWithRetry(provider, chatReq)
			if err != nil {
				log.Printf("LLM error (round %d): %v", round, err)
				writeJSON(w, ChatResponse{Error: llm.TranslateLLMError(err.Error())})
				return
			}
			continue
		}

		// --- Очистка thinking-тегов reasoning-моделей перед парсингом tool calls ---
		// Модели типа ministral-3-14b-reasoning оборачивают размышления в [THINK]...[/THINK],
		// что может помешать распознаванию JSON/XML/inline tool calls в тексте ответа.
		contentForParsing := stripThinkingTags(chatResp.Content)

		// --- Вариант 2: JSON tool call в тексте ответа (некоторые модели возвращают JSON вместо structured) ---
		// Поддерживаем оба формата: {"name":"x","arguments":{...}} и {"name":"x","parameters":{...}}
		var jsonToolCall struct {
			Name       string                 `json:"name"`
			Arguments  map[string]interface{} `json:"arguments"`
			Parameters map[string]interface{} `json:"parameters"`
		}
		if err := json.Unmarshal([]byte(strings.TrimSpace(contentForParsing)), &jsonToolCall); err == nil && jsonToolCall.Name != "" {
			toolArgs := jsonToolCall.Arguments
			if len(toolArgs) == 0 {
				toolArgs = jsonToolCall.Parameters
			}
			log.Printf("JSON tool call detected (round %d): %s, args: %+v", round, jsonToolCall.Name, toolArgs)
			messages = append(messages, llm.Message{Role: "assistant", Content: chatResp.Content})
			result := dispatchTool(req.Agent, jsonToolCall.Name, toolArgs, req.Messages)
			log.Printf("JSON tool %s called, result: %+v", jsonToolCall.Name, result)
			resultBytes, _ := json.Marshal(result)
			messages = append(messages, llm.Message{Role: "tool", Content: string(resultBytes), ToolCallID: "json-0"})
			chatReq.Messages = messages
			chatResp, err = chatWithRetry(provider, chatReq)
			if err != nil {
				log.Printf("LLM error (round %d): %v", round, err)
				writeJSON(w, ChatResponse{Error: llm.TranslateLLMError(err.Error())})
				return
			}
			continue
		}

		// --- Вариант 3: XML tool call(nemotron и подобные модели) ---
		// Формат: <tool_call><function=имя><parameter=ключ>значение</parameter></function></tool_call>
		if xmlName, xmlArgs, ok := parseXMLToolCall(contentForParsing); ok {
			log.Printf("XML tool call detected (round %d): %s, args: %+v", round, xmlName, xmlArgs)
			messages = append(messages, llm.Message{Role: "assistant", Content: chatResp.Content})
			result := dispatchTool(req.Agent, xmlName, xmlArgs, req.Messages)
			log.Printf("XML tool %s result: %+v", xmlName, result)
			resultBytes, _ := json.Marshal(result)
			messages = append(messages, llm.Message{Role: "tool", Content: string(resultBytes), ToolCallID: "xml-0"})
			chatReq.Messages = messages
			chatResp, err = chatWithRetry(provider, chatReq)
			if err != nil {
				log.Printf("LLM error (round %d): %v", round, err)
				writeJSON(w, ChatResponse{Error: llm.TranslateLLMError(err.Error())})
				return
			}
			continue
		}

		// --- Вариант 4: Inline tool call формат "toolname{json}" (devstral и подобные модели) ---
		// Некоторые модели возвращают tool call как текст: execute{"command":"ls"} вместо structured формата.
		// Парсим имя функции и JSON-аргументы из текста ответа.
		inlineRe := regexp.MustCompile(`^(\w+)(\{.+\})$`)
		trimmedContent := strings.TrimSpace(contentForParsing)
		if matches := inlineRe.FindStringSubmatch(trimmedContent); len(matches) == 3 {
			inlineName := matches[1]
			var inlineArgs map[string]interface{}
			if json.Unmarshal([]byte(matches[2]), &inlineArgs) == nil {
				log.Printf("Inline tool call detected (round %d): %s, args: %+v", round, inlineName, inlineArgs)
				messages = append(messages, llm.Message{Role: "assistant", Content: chatResp.Content})
				result := dispatchTool(req.Agent, inlineName, inlineArgs, req.Messages)
				log.Printf("Inline tool %s result: %+v", inlineName, result)
				resultBytes, _ := json.Marshal(result)
				messages = append(messages, llm.Message{Role: "tool", Content: string(resultBytes), ToolCallID: "inline-0"})
				chatReq.Messages = messages
				chatResp, err = chatWithRetry(provider, chatReq)
				if err != nil {
					log.Printf("LLM error (round %d): %v", round, err)
					writeJSON(w, ChatResponse{Error: llm.TranslateLLMError(err.Error())})
					return
				}
				continue
			}
		}

		// --- Нет tool calls— это финальный текстовый ответ ---
		break
	}

	// Очищаем финальный ответ от thinking-тегов reasoning-моделей перед отправкой пользователю
	finalContent := stripThinkingTags(chatResp.Content)
	if strings.TrimSpace(finalContent) == "" {
		log.Printf("LLM вернул пустой ответ для агента %s модели %s", req.Agent, agent.LLMModel)
		writeJSON(w, ChatResponse{Error: "Модель вернула пустой ответ. Возможно, исчерпан лимит запросов или модель недоступна. Попробуйте другую модель."})
		return
	}
	lastUserMsg := req.Messages[len(req.Messages)-1]
	saveChatMessages(req.Agent, lastUserMsg, finalContent)
	go extractAndStoreLearnings(agent.LLMModel, req.Agent, lastUserMsg.Content, finalContent)
	WriteSystemLog("info", "agent-service", fmt.Sprintf("Чат: агент=%s, модель=%s/%s", req.Agent, providerName, agent.LLMModel), fmt.Sprintf("Вопрос: %s", truncate(lastUserMsg.Content, 200)))
	statusCode = 200
	defer func() {
		metrics.RecordHTTPRequest(r.Method, "/chat", statusCode, time.Since(startTime))
	}()
	writeJSON(w, ChatResponse{Response: finalContent, Sources: ragSources})
}

// dispatchTool — единый диспетчер выполнения инструментов.
// Централизует логику маршрутизации tool calls для всех форматов (structured, JSON, XML).
// Обрабатывает специальные инструменты (call_coder, call_novice, configure_agent и др.)
// и делегирует остальные в tools-service через callTool().
//
// Параметры:
//   - agentName: имя текущего агента (для проверки прав доступа)
//   - toolName: имя вызываемого инструмента
//   - args: аргументы инструмента
//   - history: история сообщений (для делегирования задач другим агентам)
func dispatchTool(agentName, toolName string, args map[string]interface{}, history []llm.Message) map[string]interface{} {
	switch toolName {
	case "configure_agent":
		return handleConfigureAgent(args)
	case "get_agent_info":
		return handleGetAgentInfo(args)
	case "list_models_for_role":
		return handleListModelsForRole(args)
	case "view_logs":
		return handleViewLogs(args)
	case "debug_code":
		filePath, _ := args["file_path"].(string)
		cmdArgs, _ := args["args"].(string)
		cmd := filePath
		if cmdArgs != "" {
			cmd = filePath + " " + cmdArgs
		}
		result, err := callTool("execute", map[string]interface{}{"command": cmd})
		if err != nil {
			return map[string]interface{}{"error": err.Error()}
		}
		return result
	case "edit_file":
		filePath, _ := args["file_path"].(string)
		oldText, _ := args["old_text"].(string)
		newText, _ := args["new_text"].(string)
		readResult, err := callTool("read", map[string]interface{}{"path": filePath})
		if err != nil {
			return map[string]interface{}{"error": err.Error()}
		}
		content, _ := readResult["content"].(string)
		if !strings.Contains(content, oldText) {
			return map[string]interface{}{"error": "old_text не найден в файле"}
		}
		newContent := strings.Replace(content, oldText, newText, 1)
		writeResult, err := callTool("write", map[string]interface{}{"path": filePath, "content": newContent})
		if err != nil {
			return map[string]interface{}{"error": err.Error()}
		}
		return writeResult

	// ============================================================================
	// Универсальные LEGO-блоки (compound skills)
	// Каждый скил выполняет цепочку базовых инструментов за один вызов.
	// Умная модель (7B+) предпочтёт базовые инструменты.
	// Слабая модель (3B) вызовет один составной скил и получит готовый результат.
	// ============================================================================

	// БЛОК 1: Системные
	case "full_system_report":
		return handleFullSystemReport()
	case "check_stack":
		return handleCheckStack(args)
	case "diagnose_service":
		return handleDiagnoseService(args)

	// БЛОК 2: Интернет
	case "web_research":
		return handleWebResearch(args)
	case "check_resources_batch":
		return handleCheckResourcesBatch(args)

	// БЛОК 3: Файлы и отчёты
	case "generate_report":
		return handleGenerateReport(args)
	case "create_script":
		return handleCreateScript(args)

	// БЛОК 5: Утилиты
	case "run_commands":
		return handleRunCommands(args)
	case "setup_cron_job":
		return handleSetupCronJob(args)
	case "setup_git_automation":
		return handleSetupGitAutomation(args)
	case "project_init":
		return handleProjectInit(args)

	// БЛОК 6: Установка ПО
	case "install_packages":
		return handleInstallPackages(args)

	default:
		result, err := callTool(toolName, args)
		if err != nil {
			log.Printf("Tool call error: %v", err)
			return map[string]interface{}{"error": err.Error()}
		}
		return result
	}
}

// parseToolArguments — универсальный парсер аргументов tool call.
// Ollama возвращает arguments как JSON-объект, OpenRouter/OpenAI — как JSON-строку.
// Некоторые модели могут вернуть число, строку или невалидный JSON.
// Функция пытается распарсить в map[string]interface{}, а при неудаче
// оборачивает значение в map с ключом "value".
func parseToolArguments(raw json.RawMessage) map[string]interface{} {
	if len(raw) == 0 {
		return map[string]interface{}{}
	}
	// Попытка 1: стандартный JSON-объект {"key": "value"}
	var args map[string]interface{}
	if err := json.Unmarshal(raw, &args); err == nil {
		return args
	}
	// Попытка 2: JSON-строка (OpenAI формат) — "{\"key\": \"value\"}"
	var jsonStr string
	if err := json.Unmarshal(raw, &jsonStr); err == nil {
		if err2 := json.Unmarshal([]byte(jsonStr), &args); err2 == nil {
			return args
		}
		return map[string]interface{}{"value": jsonStr}
	}
	// Попытка 3: любое другое значение (число, массив и т.д.)
	var anyVal interface{}
	if err := json.Unmarshal(raw, &anyVal); err == nil {
		return map[string]interface{}{"value": anyVal}
	}
	log.Printf("Не удалось распарсить аргументы tool call: %s", string(raw))
	return map[string]interface{}{}
}

// parseXMLToolCall — парсит XML-формат tool calls от моделей типа nemotron.
// Формат: <tool_call><function=имя><parameter=ключ>значение</parameter>...</function></tool_call>
// Возвращает имя функции, аргументы и флаг успешного парсинга.
func parseXMLToolCall(content string) (string, map[string]interface{}, bool) {
	content = strings.TrimSpace(content)
	if !strings.Contains(content, "<tool_call>") {
		return "", nil, false
	}

	// Формат 1 (nemotron): <tool_call><function=имя><parameter=ключ>значение</parameter></function></tool_call>
	fnRe := regexp.MustCompile(`<function=([^>]+)>`)
	fnMatch := fnRe.FindStringSubmatch(content)
	if len(fnMatch) >= 2 {
		funcName := strings.TrimSpace(fnMatch[1])
		paramRe := regexp.MustCompile(`<parameter=([^>]+)>\s*([\s\S]*?)\s*</parameter>`)
		paramMatches := paramRe.FindAllStringSubmatch(content, -1)
		args := make(map[string]interface{})
		for _, m := range paramMatches {
			if len(m) >= 3 {
				key := strings.TrimSpace(m[1])
				val := strings.TrimSpace(m[2])
				args[key] = val
			}
		}
		return funcName, args, true
	}

	// Формат 2 (glm и др.): <tool_call>имя_функции</tool_call> или <tool_call>имя_функции{"key":"val"}</tool_call>
	simpleRe := regexp.MustCompile(`<tool_call>\s*(\w+)\s*((?:\{[\s\S]*?\})?)\s*</tool_call>`)
	simpleMatch := simpleRe.FindStringSubmatch(content)
	if len(simpleMatch) >= 2 {
		funcName := strings.TrimSpace(simpleMatch[1])
		args := make(map[string]interface{})
		if len(simpleMatch) >= 3 && strings.TrimSpace(simpleMatch[2]) != "" {
			json.Unmarshal([]byte(strings.TrimSpace(simpleMatch[2])), &args)
		}
		return funcName, args, true
	}

	return "", nil, false
}

// stripThinkingTags — удаляет блоки размышлений reasoning-моделей из текста ответа.
// Поддерживает форматы: [THINK]...[/THINK] (Ministral), <think>...</think> (DeepSeek-R1, QwQ).
// Reasoning-модели (ministral-3-14b-reasoning, deepseek-r1, qwq-32b и др.) оборачивают
// свой внутренний процесс размышления в специальные теги перед финальным ответом.
// Эти теги нужно удалить, чтобы:
//  1. Не показывать пользователю внутренние размышления модели
//  2. Не ломать парсинг tool calls (JSON/XML/inline), если модель думает перед вызовом
//
// Возвращает очищенный текст без thinking-блоков.
func stripThinkingTags(content string) string {
	thinkRe := regexp.MustCompile(`(?s)\[THINK\].*?\[/THINK\]`)
	content = thinkRe.ReplaceAllString(content, "")
	xmlThinkRe := regexp.MustCompile(`(?s)<think>.*?</think>`)
	content = xmlThinkRe.ReplaceAllString(content, "")
	return strings.TrimSpace(content)
}

// handleConfigureAgent — обработчик инструмента configure_agent.
// Позволяет Админу настраивать других агентов: менять модель, провайдера, промпт.
// Админ может подбирать правильные модели на роли Кодера и Послушника.
//
// Параметры (из args):
//   - agent_name (обязательный): имя агента для настройки
//   - model (опциональный): новая модель
//   - provider (опциональный): новый провайдер
//   - prompt (опциональный): новый системный промпт
func handleConfigureAgent(args map[string]interface{}) map[string]interface{} {
	agentName, ok := args["agent_name"].(string)
	if !ok || agentName == "" {
		return map[string]interface{}{"error": "agent_name обязателен"}
	}

	var agent models.Agent
	if err := db.DB.Where("name = ?", agentName).First(&agent).Error; err != nil {
		return map[string]interface{}{"error": "Агент не найден: " + agentName}
	}

	changes := []string{}

	if model, ok := args["model"].(string); ok && model != "" {
		agent.LLMModel = model
		changes = append(changes, "модель: "+model)
	}
	if provider, ok := args["provider"].(string); ok && provider != "" {
		agent.Provider = provider
		changes = append(changes, "провайдер: "+provider)
	}
	if prompt, ok := args["prompt"].(string); ok && prompt != "" {
		agent.Prompt = prompt
		changes = append(changes, "промпт обновлён")
	}

	if len(changes) == 0 {
		return map[string]interface{}{"error": "Не указаны параметры для изменения (model, provider, prompt)"}
	}

	if err := db.DB.Save(&agent).Error; err != nil {
		return map[string]interface{}{"error": "Ошибка сохранения: " + err.Error()}
	}

	return map[string]interface{}{
		"status":  "ok",
		"agent":   agentName,
		"changes": changes,
		"message": "Агент " + agentName + " успешно настроен",
	}
}

// handleGetAgentInfo — обработчик инструмента get_agent_info.
// Возвращает полную информацию об агенте: модель, провайдер, промпт, поддержка инструментов.
func handleGetAgentInfo(args map[string]interface{}) map[string]interface{} {
	agentName, ok := args["agent_name"].(string)
	if !ok || agentName == "" {
		return map[string]interface{}{"error": "agent_name обязателен"}
	}

	var agent models.Agent
	if err := db.DB.Where("name = ?", agentName).First(&agent).Error; err != nil {
		return map[string]interface{}{"error": "Агент не найден: " + agentName}
	}

	return map[string]interface{}{
		"name":           agent.Name,
		"model":          agent.LLMModel,
		"provider":       agent.Provider,
		"supports_tools": agent.SupportsTools,
		"prompt":         agent.Prompt,
		"prompt_file":    agent.CurrentPromptFile,
		"avatar":         agent.Avatar,
	}
}

// handleListModelsForRole — обработчик инструмента list_models_for_role.
// Возвращает список доступных моделей с рекомендациями для указанной роли.
// Показывает какие модели подходят (suitable=true) и какие нет, с пояснениями.
func handleListModelsForRole(args map[string]interface{}) map[string]interface{} {
	role, ok := args["role"].(string)
	if !ok || role == "" {
		return map[string]interface{}{"error": "role обязателен (admin, coder, novice)"}
	}

	ollamaModels, err := repository.GetOllamaModels()
	if err != nil {
		return map[string]interface{}{"error": "Не удалось получить список моделей: " + err.Error()}
	}

	type modelRec struct {
		Name     string `json:"name"`
		Suitable bool   `json:"suitable"`
		Note     string `json:"note"`
		Family   string `json:"family"`
		Size     string `json:"size"`
	}
	result := make([]modelRec, 0, len(ollamaModels))

	for _, m := range ollamaModels {
		fullInfo, infoErr := repository.GetModelFullInfo(m)
		if infoErr != nil {
			result = append(result, modelRec{Name: m, Note: "Ошибка получения информации"})
			continue
		}
		var roles []string
		var notes map[string]string
		json.Unmarshal([]byte(fullInfo.SuitableRoles), &roles)
		json.Unmarshal([]byte(fullInfo.RoleNotes), &notes)

		suitable := false
		for _, r := range roles {
			if r == role {
				suitable = true
				break
			}
		}
		result = append(result, modelRec{
			Name:     m,
			Suitable: suitable,
			Note:     notes[role],
			Family:   fullInfo.Family,
			Size:     fullInfo.ParameterSize,
		})
	}

	return map[string]interface{}{
		"role":   role,
		"models": result,
	}
}

// healthHandler — проверка состояния сервиса (GET /health).
// Возвращает JSON {"status":"ok","service":"agent-service"} для мониторинга.
func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok","service":"agent-service"}`))
}

// agentsHandler — получение списка всех агентов с их настройками (GET /agents).
// Возвращает JSON-массив с информацией о каждом агенте:
// имя, текущая модель, провайдер, поддержка инструментов, аватар, промпт.
// Используется фронтендом для отображения карточек агентов в панели моделей.
func agentsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var agents []models.Agent
	if err := db.DB.Find(&agents).Error; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var result []map[string]interface{}
	for _, a := range agents {
		result = append(result, map[string]interface{}{
			"name":          a.Name,
			"model":         a.LLMModel,
			"provider":      a.Provider,
			"supportsTools": a.SupportsTools,
			"avatar":        a.Avatar,
			"prompt_file":   a.CurrentPromptFile,
			"prompt":        a.Prompt,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	writeJSON(w, result)
}

// modelsHandler — получение списка локальных моделей Ollama (GET /models).
// Запрашивает список установленных моделей у Ollama, синхронизирует с БД
// и возвращает полную информацию о каждой модели: поддержка инструментов,
// семейство, размер, специализация на коде, подходящие роли и пояснения.
//
// Возвращает JSON-массив объектов ModelInfo для отображения в UI.
// Вся информация определяется автоматически — никаких жёстких привязок.
func modelsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ollamaModels, err := repository.GetOllamaModels()
	if err != nil {
		http.Error(w, "Failed to get models: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if err := repository.SyncModels(ollamaModels); err != nil {
		http.Error(w, "Failed to sync models: "+err.Error(), http.StatusInternalServerError)
		return
	}

	type ModelInfo struct {
		Name          string            `json:"name"`
		SupportsTools bool              `json:"supportsTools"`
		Family        string            `json:"family"`
		ParameterSize string            `json:"parameterSize"`
		IsCodeModel   bool              `json:"isCodeModel"`
		SuitableRoles []string          `json:"suitableRoles"`
		RoleNotes     map[string]string `json:"roleNotes"`
	}
	result := make([]ModelInfo, 0, len(ollamaModels))

	for _, m := range ollamaModels {
		fullInfo, err := repository.GetModelFullInfo(m)
		if err != nil {
			log.Printf("Ошибка получения информации о модели %s: %v", m, err)
			result = append(result, ModelInfo{Name: m})
			continue
		}

		var roles []string
		var notes map[string]string
		json.Unmarshal([]byte(fullInfo.SuitableRoles), &roles)
		json.Unmarshal([]byte(fullInfo.RoleNotes), &notes)

		result = append(result, ModelInfo{
			Name:          m,
			SupportsTools: fullInfo.SupportsTools,
			Family:        fullInfo.Family,
			ParameterSize: fullInfo.ParameterSize,
			IsCodeModel:   fullInfo.IsCodeModel,
			SuitableRoles: roles,
			RoleNotes:     notes,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	writeJSON(w, result)
}

// promptsHandler — получение списка файлов промптов для агента (GET /prompts?agent=...).
// Ищет файлы .txt, .prompt, .md в директории prompts/{agent}.
// Используется для отображения модального окна выбора промпта в UI.
func promptsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	agentName := r.URL.Query().Get("agent")
	if agentName == "" {
		http.Error(w, "Missing agent parameter", http.StatusBadRequest)
		return
	}
	promptsDir := filepath.Join(".", "prompts", agentName)
	if _, err := os.Stat(promptsDir); os.IsNotExist(err) {
		writeJSON(w, []string{})
		return
	}
	files, err := os.ReadDir(promptsDir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	result := []string{}
	for _, f := range files {
		if !f.IsDir() && (strings.HasSuffix(f.Name(), ".txt") || strings.HasSuffix(f.Name(), ".prompt") || strings.HasSuffix(f.Name(), ".md")) {
			result = append(result, f.Name())
		}
	}
	w.Header().Set("Content-Type", "application/json")
	writeJSON(w, result)
}

// loadPromptHandler — загрузка промпта из файла (POST /prompts/load).
// Читает содержимое файла prompts/{agent}/{filename}, обновляет промпт агента в БД.
// Устанавливает CurrentPromptFile для отображения текущего выбранного файла.
func loadPromptHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Agent    string `json:"agent"`
		Filename string `json:"filename"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Agent == "" || req.Filename == "" {
		http.Error(w, "agent and filename required", http.StatusBadRequest)
		return
	}
	agent, err := repository.GetAgentByName(req.Agent)
	if err != nil {
		http.Error(w, "Agent not found", http.StatusNotFound)
		return
	}
	promptPath := filepath.Join(".", "prompts", req.Agent, req.Filename)
	content, err := os.ReadFile(promptPath)
	if err != nil {
		http.Error(w, "Failed to read prompt file", http.StatusInternalServerError)
		return
	}
	agent.Prompt = string(content)
	agent.CurrentPromptFile = req.Filename
	if err := db.DB.Save(agent).Error; err != nil {
		http.Error(w, "Failed to update agent", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	writeJSON(w, map[string]string{"status": "ok"})
}

// updatePromptHandler — обновление промпта вручную (POST /agent/prompt).
// Устанавливает новый системный промпт, введённый пользователем через UI.
// Сбрасывает CurrentPromptFile, так как промпт больше не привязан к файлу.
func updatePromptHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req UpdatePromptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Agent == "" {
		http.Error(w, "agent required", http.StatusBadRequest)
		return
	}
	agent, err := repository.GetAgentByName(req.Agent)
	if err != nil {
		http.Error(w, "Agent not found", http.StatusNotFound)
		return
	}
	agent.Prompt = req.Prompt
	agent.CurrentPromptFile = "" // сбрасываем, так как промпт введён вручную
	if err := db.DB.Save(agent).Error; err != nil {
		http.Error(w, "Failed to update agent", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	writeJSON(w, map[string]string{"status": "ok"})
}

// updateAgentModelHandler — смена модели и/или провайдера агента (POST /update-model).
// Позволяет переключить агента на другую модель (локальную или облачную)
// и при необходимости изменить провайдера.
// Например: {"agent":"admin", "model":"gpt-4o", "provider":"openai"}
func updateAgentModelHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req UpdateModelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Agent == "" || req.Model == "" {
		http.Error(w, "agent and model required", http.StatusBadRequest)
		return
	}

	var agent models.Agent
	if err := db.DB.Where("name = ?", req.Agent).First(&agent).Error; err != nil {
		http.Error(w, "Agent not found", http.StatusNotFound)
		return
	}

	agent.LLMModel = req.Model
	if req.Provider != "" {
		agent.Provider = req.Provider
	}
	if err := db.DB.Save(&agent).Error; err != nil {
		http.Error(w, "Failed to update agent", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	writeJSON(w, map[string]string{"status": "ok"})
}

// avatarUploadHandler — загрузка аватара агента (POST /avatar?agent=...).
// Принимает multipart/form-data с файлом изображения (до 10 МБ).
// Сохраняет файл в uploads/avatars/{agent}_{filename} и обновляет поле Avatar в БД.
// Файлы раздаются через /uploads/avatars/ как статика.
func avatarUploadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	agentName := r.URL.Query().Get("agent")
	if agentName == "" {
		http.Error(w, "agent parameter required", http.StatusBadRequest)
		return
	}

	err := r.ParseMultipartForm(10 << 20) // 10 MB
	if err != nil {
		http.Error(w, "Failed to parse multipart form", http.StatusBadRequest)
		return
	}
	file, handler, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "File not provided", http.StatusBadRequest)
		return
	}
	defer file.Close()

	uploadDir := filepath.Join("uploads", "avatars")
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		http.Error(w, "Failed to create upload dir", http.StatusInternalServerError)
		return
	}

	filename := agentName + "_" + handler.Filename
	dst := filepath.Join(uploadDir, filename)

	out, err := os.Create(dst)
	if err != nil {
		http.Error(w, "Failed to save file", http.StatusInternalServerError)
		return
	}
	defer out.Close()
	if _, err := io.Copy(out, file); err != nil {
		http.Error(w, "Failed to copy file", http.StatusInternalServerError)
		return
	}
	log.Printf("Avatar saved to %s", dst)

	agent, err := repository.GetAgentByName(agentName)
	if err != nil {
		http.Error(w, "Agent not found", http.StatusNotFound)
		return
	}
	agent.Avatar = filename
	if err := db.DB.Save(agent).Error; err != nil {
		http.Error(w, "Failed to update agent avatar", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	writeJSON(w, map[string]string{"status": "ok"})
}

// avatarGetHandler — получение информации об аватаре агента (GET /avatar-info?agent=...).
// Возвращает JSON с именем файла аватара или 404, если аватар не загружен.
func avatarGetHandler(w http.ResponseWriter, r *http.Request) {
	agentName := r.URL.Query().Get("agent")
	if agentName == "" {
		http.Error(w, "agent parameter required", http.StatusBadRequest)
		return
	}
	agent, err := repository.GetAgentByName(agentName)
	if err != nil {
		http.Error(w, "Agent not found", http.StatusNotFound)
		return
	}
	if agent.Avatar == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	writeJSON(w, map[string]string{"avatar": agent.Avatar})
}

// rootHandler — обработчик корневого пути (GET /).
// Возвращает список агентов с их настройками в формате JSON.
// Используется для проверки работоспособности сервиса и отладки.
// Обрабатывает только точный путь "/" — для остальных возвращает 404.
func rootHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	var agents []models.Agent
	if err := db.DB.Find(&agents).Error; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var result []map[string]interface{}
	for _, a := range agents {
		result = append(result, map[string]interface{}{
			"name":          a.Name,
			"model":         a.LLMModel,
			"provider":      a.Provider,
			"supportsTools": a.SupportsTools,
			"avatar":        a.Avatar,
			"prompt_file":   a.CurrentPromptFile,
			"prompt":        a.Prompt,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	writeJSON(w, result)
}

// writeJSON — вспомогательная функция для JSON-ответов.
// Кодирует значение v в JSON и записывает в ResponseWriter.
// При ошибке кодирования логирует ошибку (не паникует).
// Заголовок Content-Type должен быть установлен вызывающей функцией.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("JSON encode error: %v", err)
	}
}

// saveChatMessages — сохранение пары сообщений (пользователь + агент) в PostgreSQL.
// Используется после каждого успешного ответа от LLM для персистентной истории чатов.
//
// Порядок действий:
//  1. Поиск агента в БД по имени (для привязки сообщений к агенту через AgentID)
//  2. Создание записи сообщения пользователя (role: user)
//  3. Создание записи ответа ассистента (role: assistant)
//
// При ошибке — логирует предупреждение, но не прерывает работу.
func saveChatMessages(agentName string, userMessage llm.Message, response string) {
	var agent models.Agent
	if err := db.DB.Where("name = ?", agentName).First(&agent).Error; err != nil {
		log.Printf("Failed to find agent for chat save: %v", err)
		return
	}

	role := userMessage.Role
	if role == "" {
		role = "user"
	}
	dbMsg := models.Message{
		Role:    role,
		Content: userMessage.Content,
		AgentID: agent.ID,
	}
	if err := db.DB.Create(&dbMsg).Error; err != nil {
		log.Printf("Failed to save user message: %v", err)
	}

	assistantMsg := models.Message{
		Role:    "assistant",
		Content: response,
		AgentID: agent.ID,
	}
	if err := db.DB.Create(&assistantMsg).Error; err != nil {
		log.Printf("Failed to save assistant message: %v", err)
	}
}

// fetchModelLearnings — получение релевантных знаний модели из memory-service.
// Вызывается перед каждым запросом к LLM. Найденные знания добавляются
// к системному промпту, обогащая контекст модели накопленными знаниями.
//
// Система обучения работает следующим образом:
//  1. Перед отправкой запроса к LLM берётся последнее сообщение пользователя
//  2. По нему выполняется семантический поиск в базе знаний модели (ChromaDB)
//  3. Найденные релевантные знания добавляются к системному промпту
//  4. Модель получает обогащённый контекст и может давать более точные ответы
//
// Параметры:
//   - modelName: имя модели LLM (например, "llama3.1:8b")
//   - query: текст запроса для семантического поиска (последнее сообщение пользователя)
//
// Возвращает:
//   - []string: список релевантных знаний (может быть пустым)
func fetchModelLearnings(modelName string, query string) []string {
	memoryURL := getEnv("MEMORY_SERVICE_URL", "http://localhost:8001")

	reqBody := map[string]interface{}{
		"query":      query,
		"model_name": modelName,
		"top_k":      5,
	}
	data, err := json.Marshal(reqBody)
	if err != nil {
		log.Printf("Ошибка сериализации запроса знаний: %v", err)
		return nil
	}

	resp, err := http.Post(memoryURL+"/learnings/search", "application/json", bytes.NewReader(data))
	if err != nil {
		log.Printf("Ошибка запроса знаний из memory-service: %v", err)
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("memory-service вернул статус %d при поиске знаний", resp.StatusCode)
		return nil
	}

	var result struct {
		Results   []string `json:"results"`
		Count     int      `json:"count"`
		ModelName string   `json:"model_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Printf("Ошибка декодирования ответа знаний: %v", err)
		return nil
	}

	if len(result.Results) > 0 {
		log.Printf("Найдено %d знаний для модели %s", len(result.Results), modelName)
	}
	return result.Results
}

// extractAndStoreLearnings — извлечение и сохранение знаний из диалога.
// Вызывается асинхронно (в горутине) после каждого успешного ответа от LLM.
//
// Алгоритм извлечения знаний:
//  1. Анализ последнего сообщения пользователя и ответа агента
//  2. Определение категории знания:
//     - preference: предпочтения пользователя (язык, стиль, формат)
//     - fact: факты о системе, окружении, проекте
//     - correction: исправления и уточнения от пользователя
//     - skill: успешные подходы к решению задач
//     - general: прочие полезные знания
//  3. Формирование текста знания и отправка в memory-service
//
// Знания привязываются к конкретной модели (modelName), а не к агенту,
// потому что при смене агента модель сохраняет свои знания.
//
// Параметры:
//   - modelName: имя модели LLM
//   - agentName: имя агента (admin, coder, novice)
//   - userMsg: последнее сообщение пользователя
//   - assistantResp: ответ агента
func extractAndStoreLearnings(modelName, agentName, userMsg, assistantResp string) {
	memoryURL := getEnv("MEMORY_SERVICE_URL", "http://localhost:8001")

	// Определяем категорию знания на основе содержания диалога
	category := classifyLearningCategory(userMsg, assistantResp)

	// Формируем текст знания — компактное резюме взаимодействия
	learningText := formatLearningText(userMsg, assistantResp, category)
	if learningText == "" {
		return
	}

	reqBody := map[string]interface{}{
		"text":       learningText,
		"model_name": modelName,
		"agent_name": agentName,
		"category":   category,
		"metadata": map[string]interface{}{
			"source": "auto_extract",
		},
	}
	data, err := json.Marshal(reqBody)
	if err != nil {
		log.Printf("Ошибка сериализации знания: %v", err)
		return
	}

	resp, err := http.Post(memoryURL+"/learnings", "application/json", bytes.NewReader(data))
	if err != nil {
		log.Printf("Ошибка сохранения знания в memory-service: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		log.Printf("Знание сохранено для модели %s (категория: %s)", modelName, category)
	} else {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("memory-service вернул статус %d при сохранении знания: %s", resp.StatusCode, string(body))
	}
}

// classifyLearningCategory — автоматическая классификация категории знания.
// Анализирует текст сообщения пользователя и ответа агента,
// определяя к какой категории относится извлечённое знание.
//
// Категории:
//   - "preference": пользователь выражает предпочтение (язык, стиль, формат)
//   - "correction": пользователь исправляет агента или указывает на ошибку
//   - "fact": обмен фактической информацией о системе/окружении
//   - "skill": описание решения задачи, алгоритма, подхода
//   - "general": прочие знания, не попадающие в другие категории
func classifyLearningCategory(userMsg, assistantResp string) string {
	lowerUser := strings.ToLower(userMsg)

	// Исправления и коррекции
	correctionKeywords := []string{"неправильно", "не так", "ошибка", "исправь", "wrong", "fix",
		"не верно", "неверно", "не правильно", "ты ошибся", "это не то"}
	for _, kw := range correctionKeywords {
		if strings.Contains(lowerUser, kw) {
			return "correction"
		}
	}

	// Предпочтения пользователя
	preferenceKeywords := []string{"всегда", "по-русски", "на русском", "по русски",
		"предпочитаю", "мне нравится", "хочу чтобы", "используй", "формат",
		"отвечай", "пиши", "стиль"}
	for _, kw := range preferenceKeywords {
		if strings.Contains(lowerUser, kw) {
			return "preference"
		}
	}

	// Факты о системе
	factKeywords := []string{"у меня", "мой компьютер", "моя система", "установлен",
		"версия", "конфигурация", "характеристики", "параметры", "specs"}
	for _, kw := range factKeywords {
		if strings.Contains(lowerUser, kw) {
			return "fact"
		}
	}

	// Навыки и подходы
	skillKeywords := []string{"как сделать", "решение", "алгоритм", "подход",
		"метод", "способ", "инструкция", "tutorial", "how to"}
	for _, kw := range skillKeywords {
		if strings.Contains(lowerUser, kw) {
			return "skill"
		}
	}

	return "general"
}

// formatLearningText — формирование текста знания из диалога.
// Создаёт компактное резюме взаимодействия, которое будет сохранено
// в базе знаний модели и использовано в будущих диалогах.
//
// Для разных категорий формат различается:
//   - correction: фиксируется что было неправильно и как исправлено
//   - preference: фиксируется предпочтение пользователя
//   - fact: фиксируется факт о системе/окружении
//   - skill/general: краткое резюме взаимодействия
//
// Фильтрация: слишком короткие сообщения (< 10 символов) игнорируются,
// так как из них невозможно извлечь полезное знание.
func formatLearningText(userMsg, assistantResp, category string) string {
	if len(userMsg) < 10 {
		return ""
	}

	// Ограничиваем длину для компактности
	maxUserLen := 200
	maxRespLen := 300
	if len(userMsg) > maxUserLen {
		userMsg = userMsg[:maxUserLen] + "..."
	}
	if len(assistantResp) > maxRespLen {
		assistantResp = assistantResp[:maxRespLen] + "..."
	}

	switch category {
	case "correction":
		return "Пользователь указал на ошибку: " + userMsg + " | Исправленный ответ: " + assistantResp
	case "preference":
		return "Предпочтение пользователя: " + userMsg
	case "fact":
		return "Факт о системе: " + userMsg + " | Контекст: " + assistantResp
	case "skill":
		return "Решение задачи: " + userMsg + " | Подход: " + assistantResp
	default:
		return "Контекст диалога: " + userMsg + " | Ответ: " + assistantResp
	}
}

// learningStatsHandler — получение статистики обучения из memory-service (GET /learning-stats).
// Проксирует запрос к memory-service /learnings/stats и возвращает результат.
// Показывает общее количество знаний, разбивку по моделям и категориям.
func learningStatsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	memoryURL := getEnv("MEMORY_SERVICE_URL", "http://localhost:8001")
	resp, err := http.Get(memoryURL + "/learnings/stats")
	if err != nil {
		http.Error(w, "Ошибка подключения к memory-service: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// providersHandler — управление облачными LLM-провайдерами (GET/POST /providers).
//
// GET — возвращает список всех провайдеров с их статусом:
//   - ollama (всегда включён, локальный)
//   - openai, anthropic, yandexgpt, gigachat (облачные)
//     Для каждого провайдера возвращается: имя, включён ли, есть ли API-ключ,
//     список доступных моделей (если провайдер активен).
//
// POST — сохранение/обновление конфигурации провайдера:
//
//	Принимает JSON с полями: provider, api_key, base_url, folder_id, scope, enabled.
//	Сохраняет конфигурацию в PostgreSQL и регистрирует провайдера в реестре.
//	Поля обновляются выборочно — пустые значения не перезаписывают существующие.
//
// Используется UI для настройки облачных провайдеров в панели моделей.
func providersHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		var configs []models.ProviderConfig
		db.DB.Find(&configs)

		type ProviderResponse struct {
			Name         string            `json:"name"`
			Enabled      bool              `json:"enabled"`
			Models       []string          `json:"models"`
			ModelsDetail []llm.ModelDetail `json:"models_detail"`
			HasKey       bool              `json:"hasKey"`
			Guide        ProviderGuide     `json:"guide"`
		}

		var result []ProviderResponse

		// Ollama — может быть как локальным, так и удалённым (через API)
		ollamaResp := ProviderResponse{
			Name:    "ollama",
			Enabled: true,
			HasKey:  true,
			Guide:   getProviderGuide("ollama"),
		}
		if ollamaProvider, ollamaErr := llm.GlobalRegistry.Get("ollama"); ollamaErr == nil {
			if ollamaModelList, listErr := ollamaProvider.ListModels(); listErr == nil {
				ollamaResp.Models = ollamaModelList
			}
			if ollamaDetailed, detailErr := ollamaProvider.ListModelsDetailed(); detailErr == nil {
				ollamaResp.ModelsDetail = ollamaDetailed
			}
		}
		result = append(result, ollamaResp)

		cloudProviders := []string{"yandexgpt", "gigachat"}
		for _, name := range cloudProviders {
			pr := ProviderResponse{Name: name, Guide: getProviderGuide(name)}
			for _, cfg := range configs {
				if cfg.ProviderName == name {
					pr.Enabled = cfg.Enabled
					pr.HasKey = cfg.APIKey != "" || cfg.ServiceAccountJSON != ""
					break
				}
			}
			p, err := llm.GlobalRegistry.Get(name)
			if err == nil {
				pr.HasKey = true
				modelNames, _ := p.ListModels()
				pr.Models = modelNames
				detailed, _ := p.ListModelsDetailed()
				pr.ModelsDetail = detailed
			}
			result = append(result, pr)
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		writeJSON(w, result)

	case http.MethodPost:
		var req struct {
			Provider           string `json:"provider"`
			APIKey             string `json:"api_key"`
			BaseURL            string `json:"base_url"`
			FolderID           string `json:"folder_id"`
			Scope              string `json:"scope"`
			ServiceAccountJSON string `json:"service_account_json"`
			Enabled            bool   `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}
		if req.Provider == "" {
			http.Error(w, "provider required", http.StatusBadRequest)
			return
		}

		// Шаг 1: Регистрируем провайдера временно для проверки ключа
		extra := req.FolderID
		if req.Scope != "" {
			extra = req.Scope
		}
		apiKey := req.APIKey
		saJSON := req.ServiceAccountJSON
		if apiKey == "" || saJSON == "" {
			var existingCfg models.ProviderConfig
			db.DB.Where("provider_name = ?", req.Provider).First(&existingCfg)
			if apiKey == "" {
				apiKey = existingCfg.APIKey
			}
			if saJSON == "" {
				saJSON = existingCfg.ServiceAccountJSON
			}
		}
		if apiKey == "" && saJSON == "" && req.Provider != "lmstudio" && req.Provider != "ollama" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			writeJSON(w, map[string]interface{}{
				"status": "error",
				"error":  "API-ключ или JSON сервисного аккаунта не указан",
				"hint":   getProviderHint(req.Provider),
			})
			return
		}

		if err := llm.RegisterProvider(req.Provider, apiKey, req.BaseURL, extra, saJSON); err != nil {
			log.Printf("Ошибка регистрации провайдера %s: %v", req.Provider, err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			writeJSON(w, map[string]interface{}{
				"status": "error",
				"error":  fmt.Sprintf("Не удалось зарегистрировать провайдер: %v", err),
				"hint":   getProviderHint(req.Provider),
			})
			return
		}

		// Шаг 2: Проверяем ключ — запрашиваем список моделей
		p, err := llm.GlobalRegistry.Get(req.Provider)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			writeJSON(w, map[string]interface{}{
				"status": "error",
				"error":  "Провайдер не найден после регистрации",
				"hint":   getProviderHint(req.Provider),
			})
			return
		}

		if req.Provider == "yandexgpt" {
			if yp, ok := p.(*llm.YandexGPTProvider); ok {
				if err := yp.Validate(); err != nil {
					log.Printf("Проверка провайдера %s не пройдена: %v", req.Provider, err)
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusBadRequest)
					writeJSON(w, map[string]interface{}{
						"status": "error",
						"error":  fmt.Sprintf("Ключ/настройки не прошли проверку: %v", err),
						"hint":   getProviderHint(req.Provider),
					})
					return
				}
				if resolved := yp.GetResolvedFolderID(); resolved != "" && req.FolderID == "" {
					req.FolderID = resolved
				}
			}
		}

		verifyModels, verifyErr := p.ListModels()
		if verifyErr != nil {
			log.Printf("Проверка провайдера %s не пройдена: %v", req.Provider, verifyErr)
			WriteSystemLog("error", "agent-service", fmt.Sprintf("Провайдер %s: ключ не прошёл проверку", req.Provider), verifyErr.Error())
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			writeJSON(w, map[string]interface{}{
				"status": "error",
				"error":  fmt.Sprintf("Ключ не прошёл проверку: %v", verifyErr),
				"hint":   getProviderHint(req.Provider),
			})
			return
		}
		log.Printf("Провайдер %s проверен, доступно моделей: %d", req.Provider, len(verifyModels))
		WriteSystemLog("info", "agent-service", fmt.Sprintf("Провайдер %s подключён, моделей: %d", req.Provider, len(verifyModels)), "")

		// Шаг 3: Ключ прошёл проверку — сохраняем в БД
		var cfg models.ProviderConfig
		db.DB.Where("provider_name = ?", req.Provider).FirstOrCreate(&cfg, models.ProviderConfig{ProviderName: req.Provider})
		cfg.APIKey = apiKey
		if req.BaseURL != "" {
			cfg.BaseURL = req.BaseURL
		}
		if req.FolderID != "" {
			cfg.FolderID = req.FolderID
		}
		if req.Scope != "" {
			cfg.Scope = req.Scope
		}
		if saJSON != "" {
			cfg.ServiceAccountJSON = saJSON
		}
		cfg.Enabled = req.Enabled

		if err := db.DB.Save(&cfg).Error; err != nil {
			http.Error(w, "Failed to save config", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		writeJSON(w, map[string]interface{}{
			"status": "ok",
			"models": verifyModels,
		})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// getProviderHint — возвращает подсказку для пользователя при ошибке подключения провайдера.
// Каждый провайдер имеет свои особенности аутентификации и настройки.
func getProviderHint(provider string) string {
	switch provider {
	case "gigachat":
		return "GigaChat: убедитесь, что указан корректный API-ключ (Authorization Key) из личного кабинета Сбера. " +
			"Также укажите scope: GIGACHAT_API_PERS (для физлиц) или GIGACHAT_API_B2B (для юрлиц). " +
			"Получить ключ: https://developers.sber.ru/portal/products/gigachat"
	case "yandexgpt":
		return "YandexGPT: укажите API-ключ и Folder ID из Yandex Cloud. " +
			"API-ключ создаётся в IAM → Сервисные аккаунты → Ключи API. " +
			"Folder ID находится на главной странице каталога. " +
			"Документация: https://cloud.yandex.ru/docs/yandexgpt/"
	case "openrouter":
		return "OpenRouter: укажите API-ключ с сайта https://openrouter.ai/keys. " +
			"Убедитесь, что на балансе есть средства (Credits)."
	case "openai":
		return "OpenAI: укажите API-ключ с https://platform.openai.com/api-keys. " +
			"Убедитесь, что ключ активен и на балансе есть средства."
	case "anthropic":
		return "Anthropic: укажите API-ключ с https://console.anthropic.com/settings/keys. " +
			"Убедитесь, что ключ активен."
	case "ollama":
		return "Ollama: укажите URL сервера (по умолчанию http://localhost:11434). " +
			"Убедитесь, что Ollama запущена: ollama serve. " +
			"Для удалённого доступа задайте OLLAMA_HOST=0.0.0.0"
	case "cerebras":
		return "Cerebras: укажите API-ключ с https://cloud.cerebras.ai → API Keys. " +
			"Free tier: 1M токенов/день, 30 RPM. Без привязки карты."
	default:
		return "Проверьте правильность API-ключа и параметров подключения."
	}
}

// ProviderGuide — подробное руководство по провайдеру для отображения в UI.
// Содержит пошаговые инструкции: подключение, выбор модели, оплата, проверка баланса.
type ProviderGuide struct {
	HowToConnect string `json:"how_to_connect"`
	HowToChoose  string `json:"how_to_choose"`
	HowToPay     string `json:"how_to_pay"`
	HowToBalance string `json:"how_to_balance"`
}

// getProviderGuide — возвращает подробное руководство по провайдеру.
// Инструкции включают: как подключить, как выбрать модель, где оплатить,
// как проверить баланс средств/токенов или оставшуюся подписку.
func getProviderGuide(provider string) ProviderGuide {
	switch provider {
	case "ollama":
		return ProviderGuide{
			HowToConnect: "1. Установите Ollama: curl -fsSL https://ollama.com/install.sh | sh\n" +
				"2. Запустите сервер: ollama serve\n" +
				"3. Скачайте модель: ollama pull llama3.1:8b\n" +
				"4. URL по умолчанию: http://localhost:11434\n" +
				"5. Для удалённого доступа: OLLAMA_HOST=0.0.0.0 ollama serve",
			HowToChoose: "Для роли Админ: llama3.1:8b или qwen2.5-coder:7b (поддержка tool calling).\n" +
				"Для роли Кодер: qwen2.5-coder:7b (специализация на коде).\n" +
				"Для роли Послушник: любая модель (llama3.1:8b — универсальная).",
			HowToPay: "Все модели Ollama бесплатны — работают локально на вашем ПК.\n" +
				"Оплата не требуется. Единственный ресурс — вычислительная мощность вашего GPU/CPU.",
			HowToBalance: "Ограничений по токенам нет. Проверка не требуется.\n" +
				"Для мониторинга ресурсов используйте nvidia-smi (GPU) или htop (CPU).",
		}
	case "openrouter":
		return ProviderGuide{
			HowToConnect: "1. Зарегистрируйтесь на https://openrouter.ai\n" +
				"2. Перейдите в https://openrouter.ai/keys\n" +
				"3. Нажмите 'Create Key' и скопируйте ключ (начинается с sk-or-)\n" +
				"4. Вставьте ключ в поле API Key выше",
			HowToChoose: "OpenRouter — агрегатор 200+ моделей от разных провайдеров.\n" +
				"Бесплатные модели отмечены ярким цветом (цена $0).\n" +
				"Для Админа: google/gemini-2.0-flash (бесплатная) или openai/gpt-4o.\n" +
				"Для Кодера: deepseek/deepseek-coder или anthropic/claude-3.5-sonnet.\n" +
				"Для Послушника: meta-llama/llama-3.1-8b-instruct (бесплатная).",
			HowToPay: "1. Перейдите на https://openrouter.ai/credits\n" +
				"2. Нажмите 'Add Credits'\n" +
				"3. Оплатите картой (Visa/Mastercard) от $5\n" +
				"4. Бесплатные модели (цена $0) не требуют оплаты — работают сразу.",
			HowToBalance: "Проверить баланс: https://openrouter.ai/credits\n" +
				"История использования: https://openrouter.ai/activity\n" +
				"Лимиты по ключу: https://openrouter.ai/keys → Edit Key → Credit Limit.",
		}
	case "gigachat":
		return ProviderGuide{
			HowToConnect: "1. Зарегистрируйтесь на https://developers.sber.ru\n" +
				"2. Создайте проект в Личном кабинете\n" +
				"3. Получите 'Authorization Key' (Client Credentials)\n" +
				"4. Вставьте ключ в поле API Key\n" +
				"5. Укажите Scope: GIGACHAT_API_PERS (для физлиц) или GIGACHAT_API_B2B (для юрлиц)",
			HowToChoose: "GigaChat Lite — быстрая, подходит для простых задач (Послушник).\n" +
				"GigaChat Pro — сбалансированная (Кодер).\n" +
				"GigaChat Max — самая мощная, для сложных задач (Админ).\n" +
				"Все модели поддерживают русский язык на высшем уровне.",
			HowToPay: "Физлица (GIGACHAT_API_PERS):\n" +
				"- Бесплатный тариф: 1 000 000 токенов GigaChat Lite в месяц.\n" +
				"- Платный: от 500 руб/мес в личном кабинете.\n\n" +
				"Юрлица (GIGACHAT_API_B2B):\n" +
				"- Подписка через менеджера Сбера.\n" +
				"- Оплата: https://developers.sber.ru/portal/products/gigachat → Тарифы.",
			HowToBalance: "Проверить остаток токенов: Личный кабинет → https://developers.sber.ru → Мои проекты → Статистика.\n" +
				"Подписка обновляется ежемесячно. Дата следующего обновления видна в разделе 'Подписка'.\n" +
				"При исчерпании лимита — ответы с кодом 429 (Too Many Requests).",
		}
	case "yandexgpt":
		return ProviderGuide{
			HowToConnect: "1. Зарегистрируйтесь в Yandex Cloud: https://cloud.yandex.ru\n" +
				"2. Создайте каталог (Folder) — запомните Folder ID\n" +
				"3. Создайте сервисный аккаунт с ролью ai.languageModels.user\n" +
				"4. Создайте API-ключ: IAM → Сервисные аккаунты → Создать ключ API\n" +
				"5. Вставьте API Key и Folder ID в поля выше",
			HowToChoose: "yandexgpt-lite — быстрая и дешёвая, для простых задач (Послушник).\n" +
				"yandexgpt — полная модель, для сложных задач (Админ, Кодер).\n" +
				"yandexgpt-32k — расширенный контекст 32K токенов, для больших документов.\n" +
				"summarization — специализированная модель для суммаризации текстов.",
			HowToPay: "При регистрации выдаётся грант на 4 000 руб. (действует 60 дней).\n" +
				"После гранта: оплата по факту использования.\n" +
				"Привязать карту: https://console.cloud.yandex.ru/billing → Способ оплаты.\n" +
				"Цены: yandexgpt-lite — 0.20 руб/1K токенов, yandexgpt — 1.20 руб/1K токенов.",
			HowToBalance: "Проверить баланс: https://console.cloud.yandex.ru/billing\n" +
				"Остаток гранта: Billing → Гранты → Текущий грант.\n" +
				"История расходов: Billing → Детализация → Фильтр по сервису 'YandexGPT'.\n" +
				"Настроить оповещения: Billing → Бюджеты → Создать бюджет.",
		}
	case "openai":
		return ProviderGuide{
			HowToConnect: "1. Зарегистрируйтесь на https://platform.openai.com\n" +
				"2. Перейдите в https://platform.openai.com/api-keys\n" +
				"3. Нажмите 'Create new secret key' и скопируйте ключ (начинается с sk-)\n" +
				"4. Вставьте ключ в поле API Key выше",
			HowToChoose: "gpt-4o — лучшая модель, для Админа (мультимодальная, быстрая).\n" +
				"gpt-4o-mini — дешевле, для Кодера (хорошее соотношение цены/качества).\n" +
				"gpt-3.5-turbo — самая дешёвая, для Послушника.\n" +
				"o1 / o3 — модели с 'размышлением', для сложных логических задач.",
			HowToPay: "1. Перейдите на https://platform.openai.com/settings/organization/billing\n" +
				"2. Нажмите 'Add payment method' → привяжите карту (Visa/Mastercard)\n" +
				"3. Нажмите 'Add to credit balance' → от $5\n" +
				"4. Новым пользователям даётся $5 бесплатного кредита (действует 3 месяца).",
			HowToBalance: "Проверить баланс: https://platform.openai.com/settings/organization/billing\n" +
				"Текущее использование: https://platform.openai.com/usage\n" +
				"Настроить лимиты: Settings → Limits → Monthly budget.\n" +
				"При нулевом балансе — ответы с кодом 429 (Rate limit exceeded).",
		}
	case "anthropic":
		return ProviderGuide{
			HowToConnect: "1. Зарегистрируйтесь на https://console.anthropic.com\n" +
				"2. Перейдите в https://console.anthropic.com/settings/keys\n" +
				"3. Нажмите 'Create Key' и скопируйте ключ (начинается с sk-ant-)\n" +
				"4. Вставьте ключ в поле API Key выше",
			HowToChoose: "claude-sonnet-4 — новейшая модель, баланс цены/качества (Админ).\n" +
				"claude-3.5-sonnet — отличная для кода (Кодер).\n" +
				"claude-3.5-haiku — быстрая и дешёвая (Послушник).\n" +
				"claude-3-opus — самая мощная, для самых сложных задач.",
			HowToPay: "1. Перейдите на https://console.anthropic.com/settings/billing\n" +
				"2. Нажмите 'Add payment method' → привяжите карту\n" +
				"3. Пополните баланс (минимум $5)\n" +
				"4. Новым пользователям даётся $5 бесплатного кредита.",
			HowToBalance: "Проверить баланс: https://console.anthropic.com/settings/billing\n" +
				"Текущее использование: https://console.anthropic.com/settings/usage\n" +
				"Настроить лимиты: Settings → Plans → Spend limits.\n" +
				"При нулевом балансе — ответы с кодом 400 (Insufficient credits).",
		}
	case "routeway":
		return ProviderGuide{
			HowToConnect: "1. Зарегистрируйтесь на https://routeway.ai\n" +
				"2. Перейдите в Dashboard → API Keys\n" +
				"3. Создайте ключ и скопируйте его\n" +
				"4. Вставьте ключ в поле API Key",
			HowToChoose: "Routeway — агрегатор 70+ моделей, 200 бесплатных запросов/день.\n" +
				"Бесплатные модели: llama-3.3-70b-instruct:free, deepseek-r1:free, qwen2.5-72b:free.\n" +
				"Для Админа: llama-3.3-70b-instruct:free (бесплатная, tool calling).\n" +
				"Для Кодера: qwen2.5-coder-32b:free.\n" +
				"Для Послушника: llama-3.1-8b-instruct:free.",
			HowToPay: "Бесплатные модели (с суффиксом :free) не требуют оплаты.\n" +
				"Лимит: 200 запросов в день (в 4 раза больше OpenRouter).",
			HowToBalance: "Лимит сбрасывается ежедневно.\n" +
				"Проверьте остаток запросов в Dashboard на https://routeway.ai.",
		}
	case "cerebras":
		return ProviderGuide{
			HowToConnect: "1. Зарегистрируйтесь на https://cloud.cerebras.ai\n" +
				"2. Перейдите в API Keys (левое меню)\n" +
				"3. Нажмите 'Create API Key', дайте имя и скопируйте ключ (csk-...)\n" +
				"4. Вставьте ключ в поле API Key выше",
			HowToChoose: "Cerebras — сверхбыстрый инференс (до 2500 токенов/сек, 20x быстрее OpenAI).\n" +
				"llama3.1-8b — быстрая и лёгкая, для Послушника.\n" +
				"llama-3.3-70b — мощная, для Админа и Кодера.\n" +
				"qwen-3-32b — сбалансированная (32B параметров).\n" +
				"qwen-3-235b-a22b-instruct — самая мощная (MoE, 235B параметров).\n" +
				"gpt-oss-120b — открытая GPT-модель (120B).\n" +
				"zai-glm-4.7 — Preview модель.",
			HowToPay: "Free tier (без карты):\n" +
				"- 1 000 000 токенов в день\n" +
				"- 30 запросов в минуту\n\n" +
				"PayGo (с картой):\n" +
				"- llama3.1-8b: $0.10/1M токенов\n" +
				"- llama-3.3-70b: $0.60/1M токенов\n" +
				"- qwen-3-32b: $0.30/1M токенов\n" +
				"- qwen-3-235b: $0.90/1M токенов\n" +
				"Оплата: https://cloud.cerebras.ai → Billing.",
			HowToBalance: "Проверить использование: https://cloud.cerebras.ai → Usage.\n" +
				"Free tier сбрасывается ежедневно.\n" +
				"При превышении лимита — ответы с кодом 429 (Rate limit exceeded).",
		}
	case "lmstudio":
		return ProviderGuide{
			HowToConnect: "1. Скачайте LM Studio: https://lmstudio.ai\n" +
				"2. Установите и запустите приложение\n" +
				"3. Скачайте модель (My Models → View All → поиск → Download)\n" +
				"4. Включите Developer mode: Settings (⚙) → Developer → ON\n" +
				"5. Загрузите модель в память: выберите модель → Load Model\n" +
				"6. Сервер запустится автоматически на http://localhost:1234/v1\n" +
				"7. API Key не требуется (оставьте пустым)\n" +
				"8. Нажмите кнопку ↻ (обновить) в панели провайдеров для загрузки списка моделей",
			HowToChoose: "LM Studio — бесплатные локальные модели, без лимитов запросов.\n" +
				"Для Админа: ministral-3-14b-reasoning (14B, reasoning + tool calling).\n" +
				"Для Кодера: qwen2.5-coder-14b-instruct (14B, специализация на коде).\n" +
				"Для Послушника: llama-3.1-8b-instruct (8B, универсальная).\n" +
				"Требования: минимум 10GB RAM для 14B, 8GB для 8B моделей.",
			HowToPay: "Полностью бесплатно! Модели работают локально на вашем ПК.\n" +
				"Никаких лимитов, никаких подписок, данные не покидают компьютер.",
			HowToBalance: "Ограничений нет. Проверьте ресурсы через nvidia-smi (GPU) или htop (CPU/RAM).\n" +
				"Если модель медленная — попробуйте меньшую (8B вместо 14B).",
		}
	default:
		return ProviderGuide{
			HowToConnect: "Проверьте правильность API-ключа и параметров подключения.",
			HowToChoose:  "Выберите модель, подходящую для вашей задачи.",
			HowToPay:     "Уточните условия оплаты на сайте провайдера.",
			HowToBalance: "Проверьте баланс в личном кабинете провайдера.",
		}
	}
}

// cloudModelsHandler — получение списка моделей облачного провайдера (GET /cloud-models).
// Если передан параметр ?provider=..., возвращает модели конкретного провайдера.
// Если параметр не передан — возвращает список всех зарегистрированных провайдеров.
//
// Используется фронтендом для заполнения выпадающего списка облачных моделей.
func cloudModelsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	providerName := r.URL.Query().Get("provider")
	log.Printf("[cloudModelsHandler] providerName=%s", providerName)
	if providerName == "" {
		allProviders := llm.GlobalRegistry.ListAll()
		w.Header().Set("Content-Type", "application/json")
		writeJSON(w, allProviders)
		return
	}

	p, err := llm.GlobalRegistry.Get(providerName)
	if err != nil {
		log.Printf("[cloudModelsHandler] provider not found: %v", err)
		http.Error(w, "Provider not found: "+err.Error(), http.StatusNotFound)
		return
	}
	log.Printf("[cloudModelsHandler] found provider: %T", p)
	models, err := p.ListModels()
	if err != nil {
		log.Printf("[cloudModelsHandler] ListModels error: %v", err)
		http.Error(w, "Failed to get models: "+err.Error(), http.StatusInternalServerError)
		return
	}
	log.Printf("[cloudModelsHandler] models count: %d", len(models))
	w.Header().Set("Content-Type", "application/json")
	writeJSON(w, models)
}

// workspacesHandler — управление рабочими пространствами (GET/POST/DELETE /workspaces).
//
// Рабочие пространства (Workspaces) — это изолированные проекты с:
//   - Именем и путём к рабочей директории на ПК
//   - Отдельной историей чатов
//   - Отдельной конфигурацией агентов
//
// GET    — возвращает список всех пространств
// POST   — создаёт новое пространство (JSON: {name, path})
// DELETE — удаляет пространство по ID (?id=...)
func workspacesHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		var workspaces []models.Workspace
		db.DB.Find(&workspaces)
		w.Header().Set("Content-Type", "application/json")
		writeJSON(w, workspaces)

	case http.MethodPost:
		var req struct {
			Name string `json:"name"`
			Path string `json:"path"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}
		if req.Name == "" {
			http.Error(w, "name required", http.StatusBadRequest)
			return
		}
		ws := models.Workspace{Name: req.Name, Path: req.Path}
		if err := db.DB.Create(&ws).Error; err != nil {
			http.Error(w, "Failed to create workspace", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		writeJSON(w, ws)

	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "id required", http.StatusBadRequest)
			return
		}
		if err := db.DB.Delete(&models.Workspace{}, id).Error; err != nil {
			http.Error(w, "Failed to delete workspace", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		writeJSON(w, map[string]string{"status": "ok"})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// WriteSystemLog — записывает событие в централизованную систему логов.
// Используется всеми компонентами для логирования ошибок и важных событий.
// Параметры:
//   - level: уровень лога (error, warn, info, debug)
//   - service: имя микросервиса-источника
//   - message: текст сообщения
//   - details: дополнительные данные (стек, параметры запроса)
func WriteSystemLog(level, service, message, details string) {
	entry := models.SystemLog{
		Level:   level,
		Service: service,
		Message: message,
		Details: details,
	}
	if err := db.DB.Create(&entry).Error; err != nil {
		log.Printf("Ошибка записи в системный лог: %v", err)
	}
}

// logsHandler — HTTP-обработчик для работы с системными логами.
// GET: возвращает логи с фильтрацией по уровню (?level=error), сервису (?service=agent-service),
//
//	и лимиту (?limit=100). По умолчанию возвращает последние 100 записей.
//
// POST: принимает новый лог от внешних сервисов (tools-service, memory-service, api-gateway).
// PATCH: отмечает лог как исправленный (?id=123&resolved=true).
func logsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		query := db.DB.Model(&models.SystemLog{}).Order("created_at DESC")

		if level := r.URL.Query().Get("level"); level != "" {
			query = query.Where("level = ?", level)
		}
		if service := r.URL.Query().Get("service"); service != "" {
			query = query.Where("service = ?", service)
		}
		if resolved := r.URL.Query().Get("resolved"); resolved != "" {
			query = query.Where("resolved = ?", resolved == "true")
		}

		limit := 100
		if l := r.URL.Query().Get("limit"); l != "" {
			if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
				limit = parsed
			}
		}
		query = query.Limit(limit)

		var logs []models.SystemLog
		query.Find(&logs)

		w.Header().Set("Content-Type", "application/json")
		writeJSON(w, logs)

	case http.MethodPost:
		var req struct {
			Level   string `json:"level"`
			Service string `json:"service"`
			Message string `json:"message"`
			Details string `json:"details"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}
		if req.Level == "" || req.Service == "" || req.Message == "" {
			http.Error(w, "level, service, message обязательны", http.StatusBadRequest)
			return
		}
		WriteSystemLog(req.Level, req.Service, req.Message, req.Details)
		w.Header().Set("Content-Type", "application/json")
		writeJSON(w, map[string]string{"status": "ok"})

	case http.MethodPatch:
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "id обязателен", http.StatusBadRequest)
			return
		}
		resolved := r.URL.Query().Get("resolved") == "true"
		if err := db.DB.Model(&models.SystemLog{}).Where("id = ?", id).Update("resolved", resolved).Error; err != nil {
			http.Error(w, "Ошибка обновления", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		writeJSON(w, map[string]string{"status": "ok"})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// ragAddHandler — обработчик для добавления документа в RAG базу знаний
func ragAddHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Title   string `json:"title"`
		Content string `json:"content"`
		Source  string `json:"source"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if req.Title == "" || req.Content == "" {
		http.Error(w, "title and content required", http.StatusBadRequest)
		return
	}

	docID := fmt.Sprintf("doc-%d", time.Now().UnixNano())

	if ragRetriever != nil && ragRetriever.Config().ChromaURL != "" {
		ragDoc := rag.RagDoc{
			ID:      docID,
			Title:   req.Title,
			Content: req.Content,
			Source:  req.Source,
		}
		if err := ragRetriever.AddDocument(ragDoc); err != nil {
			log.Printf("[RAG] Ошибка добавления в ChromA: %v", err)
		}
	}

	ragDoc := models.RagDocument{
		Title:       req.Title,
		Content:     req.Content,
		Source:      req.Source,
		ChunkIndex:  0,
		TotalChunks: 1,
	}
	if err := db.DB.Create(&ragDoc).Error; err != nil {
		log.Printf("[RAG] Ошибка сохранения в БД: %v", err)
		http.Error(w, "Failed to save document", http.StatusInternalServerError)
		return
	}

	log.Printf("[RAG] Добавлен документ: %s (ID: %d)", req.Title, ragDoc.ID)
	writeJSON(w, map[string]interface{}{"status": "ok", "id": docID, "db_id": ragDoc.ID})
}

// ragSearchHandler — обработчик для поиска по RAG базе знаний
func ragSearchHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	query := r.URL.Query().Get("q")
	if query == "" && r.Method == http.MethodPost {
		var req struct {
			Query string `json:"query"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
			query = req.Query
		}
	}

	if query == "" {
		http.Error(w, "query required", http.StatusBadRequest)
		return
	}

	if ragRetriever == nil {
		http.Error(w, "RAG not initialized", http.StatusInternalServerError)
		return
	}

	results, err := ragRetriever.Search(query, 5)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, results)
}

// ragFilesHandler — обработчик для получения списка файлов в RAG (сгруппировано по папкам)
func ragFilesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		var docs []models.RagDocument
		db.DB.Find(&docs)

		type fileInfo struct {
			FileName    string `json:"file_name"`
			ChunksCount int    `json:"chunks_count"`
		}
		type folderData struct {
			Folder     string     `json:"folder"`
			Files      []fileInfo `json:"files"`
			TotalFiles int        `json:"total_files"`
		}

		// Группируем по папкам
		folderMap := make(map[string]*folderData)
		for _, doc := range docs {
			// Извлекаем папку из имени файла
			parts := strings.Split(doc.Title, "/")
			var folder, fileName string
			if len(parts) > 1 {
				folder = parts[0]
				fileName = strings.Join(parts[1:], "/")
			} else {
				folder = "(корневая папка)"
				fileName = doc.Title
			}

			if _, ok := folderMap[folder]; !ok {
				folderMap[folder] = &folderData{
					Folder:     folder,
					Files:      []fileInfo{},
					TotalFiles: 0,
				}
			}

			folderMap[folder].Files = append(folderMap[folder].Files, fileInfo{
				FileName:    fileName,
				ChunksCount: doc.TotalChunks,
			})
			folderMap[folder].TotalFiles++
		}

		// Сортируем папки
		folders := make([]*folderData, 0, len(folderMap))
		for _, v := range folderMap {
			folders = append(folders, v)
		}

		// Сортируем: корневая в конце
		sort.Slice(folders, func(i, j int) bool {
			f1 := folders[i].Folder
			f2 := folders[j].Folder
			if f1 == "(корневая папка)" {
				return false
			}
			if f2 == "(корневая папка)" {
				return true
			}
			return f1 < f2
		})

		writeJSON(w, folders)
		return
	}
	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

// ragStatsHandler — обработчик для получения статистики RAG
func ragStatsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var docCount int64
	var uniqueFiles int64
	db.DB.Model(&models.RagDocument{}).Count(&docCount)
	db.DB.Model(&models.RagDocument{}).Distinct("title").Count(&uniqueFiles)

	writeJSON(w, map[string]interface{}{
		"facts_count": docCount,
		"files_count": uniqueFiles,
	})
}

// ragDeleteHandler — обработчик для удаления документа из RAG
func ragDeleteHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete && r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	fileName := r.URL.Query().Get("name")
	if fileName == "" {
		var req struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
			fileName = req.Name
		}
	}

	if fileName == "" {
		http.Error(w, "name required", http.StatusBadRequest)
		return
	}

	if err := db.DB.Where("title = ?", fileName).Delete(&models.RagDocument{}).Error; err != nil {
		log.Printf("[RAG] Ошибка удаления: %v", err)
		http.Error(w, "Failed to delete", http.StatusInternalServerError)
		return
	}

	log.Printf("[RAG] Удалён документ: %s", fileName)
	writeJSON(w, map[string]string{"status": "ok"})
}

var supportedExtensions = map[string]bool{
	".txt": true, ".md": true, ".markdown": true,
	".json": true, ".jsonl": true,
	".csv":  true,
	".html": true, ".htm": true,
	".xml":  true,
	".yaml": true, ".yml": true,
	".go": true, ".py": true, ".js": true, ".ts": true,
	".java": true, ".c": true, ".cpp": true, ".h": true, ".hpp": true,
	".rs": true, ".rb": true, ".php": true, ".swift": true, ".kt": true,
	".sh": true, ".bash": true, ".zsh": true,
	".sql": true, ".graphql": true, ".gql": true,
	".dockerfile": true, ".toml": true, ".ini": true, ".conf": true,
	".log": true,
}

// ragAddFolderHandler — обработчик для рекурсивной загрузки папки в RAG
func ragAddFolderHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		FolderPath string `json:"folder_path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	folderPath := req.FolderPath
	if folderPath == "" {
		http.Error(w, "folder_path required", http.StatusBadRequest)
		return
	}

	// Проверяем, что папка существует
	info, err := os.Stat(folderPath)
	if err != nil {
		http.Error(w, "Folder not found: "+err.Error(), http.StatusNotFound)
		return
	}
	if !info.IsDir() {
		http.Error(w, "Path is not a folder", http.StatusBadRequest)
		return
	}

	// Рекурсивно сканируем папку
	var filesAdded int
	var filesSkipped int
	var errors []string

	var walkFunc func(path string, info os.FileInfo, err error) error
	walkFunc = func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			// Пропускаем скрытые папки
			if strings.HasPrefix(info.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if !supportedExtensions[ext] {
			filesSkipped++
			return nil
		}

		// Читаем содержимое файла
		content, err := os.ReadFile(path)
		if err != nil {
			errors = append(errors, path+": "+err.Error())
			return nil
		}

		// Относительный путь от папки
		relPath, _ := filepath.Rel(folderPath, path)
		title := relPath

		docID := fmt.Sprintf("doc-%d-%s", time.Now().UnixNano(), strings.ReplaceAll(relPath, "/", "-"))

		if ragRetriever != nil && ragRetriever.Config().ChromaURL != "" {
			ragDoc := rag.RagDoc{
				ID:      docID,
				Title:   title,
				Content: string(content),
				Source:  "folder:" + folderPath,
			}
			if err := ragRetriever.AddDocument(ragDoc); err != nil {
				log.Printf("[RAG] Ошибка добавления в ChromA: %v", err)
			}
		}

		ragDoc := models.RagDocument{
			Title:       title,
			Content:     string(content),
			Source:      "folder:" + folderPath,
			ChunkIndex:  0,
			TotalChunks: 1,
		}
		if err := db.DB.Create(&ragDoc).Error; err != nil {
			log.Printf("[RAG] Ошибка сохранения в БД: %v", err)
			errors = append(errors, title+": "+err.Error())
			return nil
		}

		filesAdded++
		log.Printf("[RAG] Добавлен файл из папки: %s", title)
		return nil
	}

	if err := filepath.Walk(folderPath, walkFunc); err != nil {
		log.Printf("[RAG] Ошибка сканирования папки: %v", err)
	}

	writeJSON(w, map[string]interface{}{
		"status":        "ok",
		"folder_path":   folderPath,
		"files_added":   filesAdded,
		"files_skipped": filesSkipped,
		"errors":        errors,
	})
}

// handleViewLogs — обработчик инструмента view_logs для Админа.
// Позволяет агенту просматривать системные логи с фильтрацией по уровню и сервису.
func handleViewLogs(args map[string]interface{}) map[string]interface{} {
	query := db.DB.Model(&models.SystemLog{}).Order("created_at DESC")

	if level, ok := args["level"].(string); ok && level != "" {
		query = query.Where("level = ?", level)
	}
	if service, ok := args["service"].(string); ok && service != "" {
		query = query.Where("service = ?", service)
	}

	limit := 20
	if l, ok := args["limit"].(float64); ok && l > 0 {
		limit = int(l)
	}
	query = query.Limit(limit)

	var logs []models.SystemLog
	query.Find(&logs)

	var entries []map[string]interface{}
	for _, l := range logs {
		entries = append(entries, map[string]interface{}{
			"id":       l.ID,
			"level":    l.Level,
			"service":  l.Service,
			"message":  l.Message,
			"details":  l.Details,
			"resolved": l.Resolved,
			"time":     l.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	return map[string]interface{}{
		"count": len(entries),
		"logs":  entries,
	}
}

// ============================================================================
// Составные скилы-подстраховки (compound skill handlers)
// Каждый обработчик выполняет цепочку базовых инструментов за один вызов.
// Умная модель (7B+) предпочтёт базовые инструменты и сама построит цепочку.
// Слабая модель (3B) вызовет один составной скил и получит готовый результат.
// ============================================================================

// handleSetupGitAutomation — составной скил: полная git-автоматизация проекта.
// Выполняет цепочку: mkdir → git init → создание autocommit.sh → создание backup.sh → добавление в crontab.
// Все шаги выполняются последовательно через callTool("execute", ...).
func handleSetupGitAutomation(args map[string]interface{}) map[string]interface{} {
	projectPath, _ := args["project_path"].(string)
	backupPath, _ := args["backup_path"].(string)
	if projectPath == "" || backupPath == "" {
		return map[string]interface{}{"error": "project_path и backup_path обязательны"}
	}

	autocommitMin := 30
	if m, ok := args["autocommit_minutes"].(float64); ok && m > 0 {
		autocommitMin = int(m)
	}
	backupSchedule := "0 0 * * *"
	if s, ok := args["backup_schedule"].(string); ok && s != "" {
		backupSchedule = s
	}

	var steps []map[string]interface{}

	// Шаг 1: Создание директорий
	r1, _ := callTool("execute", map[string]interface{}{"command": fmt.Sprintf("mkdir -p %s %s", projectPath, backupPath)})
	steps = append(steps, map[string]interface{}{"step": "mkdir", "result": r1})

	// Шаг 2: Инициализация git
	r2, _ := callTool("execute", map[string]interface{}{"command": fmt.Sprintf("cd %s && git init && git config user.email 'admin@openclaw.local' && git config user.name 'OpenClaw Admin'", projectPath)})
	steps = append(steps, map[string]interface{}{"step": "git_init", "result": r2})

	// Шаг 3: Создание autocommit.sh
	autocommitScript := fmt.Sprintf("#!/bin/bash\n# Автоматический коммит всех изменений в проекте\n# Создан составным скилом setup_git_automation\ncd %s\ngit add -A\nDATETIME=$(date '+%%Y-%%m-%%d %%H:%%M:%%S')\ngit diff --cached --quiet || git commit -m \"auto-commit: $DATETIME\"\n", projectPath)
	autocommitPath := projectPath + "/autocommit.sh"
	r3, _ := callTool("write", map[string]interface{}{"path": autocommitPath, "content": autocommitScript})
	steps = append(steps, map[string]interface{}{"step": "write_autocommit", "result": r3})

	r3b, _ := callTool("execute", map[string]interface{}{"command": "chmod +x " + autocommitPath})
	steps = append(steps, map[string]interface{}{"step": "chmod_autocommit", "result": r3b})

	// Шаг 4: Создание backup.sh
	backupScript := fmt.Sprintf("#!/bin/bash\n# Резервное копирование проекта\n# Создан составным скилом setup_git_automation\nDATETIME=$(date '+%%Y%%m%%d_%%H%%M%%S')\nmkdir -p %s\ntar -czf %s/backup_${DATETIME}.tar.gz -C %s .\necho \"Бэкап создан: %s/backup_${DATETIME}.tar.gz\"\n", backupPath, backupPath, projectPath, backupPath)
	backupScriptPath := projectPath + "/backup.sh"
	r4, _ := callTool("write", map[string]interface{}{"path": backupScriptPath, "content": backupScript})
	steps = append(steps, map[string]interface{}{"step": "write_backup", "result": r4})

	r4b, _ := callTool("execute", map[string]interface{}{"command": "chmod +x " + backupScriptPath})
	steps = append(steps, map[string]interface{}{"step": "chmod_backup", "result": r4b})

	// Шаг 5: Добавление в crontab
	cronCmd := fmt.Sprintf("(crontab -l 2>/dev/null; echo '*/%d * * * * %s'; echo '%s %s') | sort -u | crontab -", autocommitMin, autocommitPath, backupSchedule, backupScriptPath)
	r5, _ := callTool("execute", map[string]interface{}{"command": cronCmd})
	steps = append(steps, map[string]interface{}{"step": "crontab", "result": r5})

	// Шаг 6: Первый коммит
	r6, _ := callTool("execute", map[string]interface{}{"command": fmt.Sprintf("cd %s && git add -A && git commit -m 'init: проект создан с автоматизацией'", projectPath)})
	steps = append(steps, map[string]interface{}{"step": "initial_commit", "result": r6})

	// Шаг 7: Проверка crontab
	r7, _ := callTool("execute", map[string]interface{}{"command": "crontab -l"})
	steps = append(steps, map[string]interface{}{"step": "verify_crontab", "result": r7})

	return map[string]interface{}{
		"success":     true,
		"message":     fmt.Sprintf("Git-автоматизация настроена: проект %s, бэкапы в %s, автокоммит каждые %d мин, бэкап по расписанию %s", projectPath, backupPath, autocommitMin, backupSchedule),
		"steps_count": len(steps),
		"steps":       steps,
	}
}

// handleFullSystemReport — составной скил: полный отчёт о системе.
// Собирает данные из sysinfo + sysload + cputemp + df + free + uname за один вызов.
func handleFullSystemReport() map[string]interface{} {
	report := make(map[string]interface{})

	if r, err := callTool("sysinfo", map[string]interface{}{}); err == nil {
		report["sysinfo"] = r
	}
	if r, err := callTool("sysload", map[string]interface{}{}); err == nil {
		report["sysload"] = r
	}
	if r, err := callTool("cputemp", map[string]interface{}{}); err == nil {
		report["cputemp"] = r
	}
	if r, err := callTool("execute", map[string]interface{}{"command": "df -h"}); err == nil {
		report["disk"] = r
	}
	if r, err := callTool("execute", map[string]interface{}{"command": "free -m"}); err == nil {
		report["memory"] = r
	}
	if r, err := callTool("execute", map[string]interface{}{"command": "uname -a"}); err == nil {
		report["kernel"] = r
	}

	report["success"] = true
	report["message"] = "Полный системный отчёт собран"
	return report
}

// handleRunCommands — составной скил: последовательное выполнение нескольких bash-команд.
// Принимает массив команд, выполняет каждую через callTool("execute") и собирает результаты.
func handleRunCommands(args map[string]interface{}) map[string]interface{} {
	commandsRaw, ok := args["commands"]
	if !ok {
		return map[string]interface{}{"error": "commands обязателен"}
	}

	var commands []string
	switch v := commandsRaw.(type) {
	case []interface{}:
		for _, c := range v {
			if s, ok := c.(string); ok {
				commands = append(commands, s)
			}
		}
	case []string:
		commands = v
	default:
		return map[string]interface{}{"error": "commands должен быть массивом строк"}
	}

	if len(commands) == 0 {
		return map[string]interface{}{"error": "commands пуст"}
	}

	var results []map[string]interface{}
	allOk := true
	for i, cmd := range commands {
		r, err := callTool("execute", map[string]interface{}{"command": cmd})
		entry := map[string]interface{}{
			"index":   i,
			"command": cmd,
		}
		if err != nil {
			entry["error"] = err.Error()
			allOk = false
		} else {
			entry["result"] = r
		}
		results = append(results, entry)
	}

	return map[string]interface{}{
		"success":  allOk,
		"message":  fmt.Sprintf("Выполнено %d команд", len(results)),
		"commands": results,
	}
}

// handleCreateScript — составной скил: создание исполняемого bash-скрипта.
// Записывает содержимое в файл и делает chmod +x за один вызов.
func handleCreateScript(args map[string]interface{}) map[string]interface{} {
	path, _ := args["path"].(string)
	content, _ := args["content"].(string)
	if path == "" || content == "" {
		return map[string]interface{}{"error": "path и content обязательны"}
	}

	// Создаём директорию если нужно
	dir := path[:strings.LastIndex(path, "/")]
	if dir != "" {
		callTool("execute", map[string]interface{}{"command": "mkdir -p " + dir})
	}

	// Записываем файл
	writeResult, err := callTool("write", map[string]interface{}{"path": path, "content": content})
	if err != nil {
		return map[string]interface{}{"error": "Ошибка записи: " + err.Error()}
	}

	// Делаем исполняемым
	chmodResult, err := callTool("execute", map[string]interface{}{"command": "chmod +x " + path})
	if err != nil {
		return map[string]interface{}{"error": "Ошибка chmod: " + err.Error()}
	}

	return map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Скрипт создан: %s (chmod +x)", path),
		"write":   writeResult,
		"chmod":   chmodResult,
	}
}

// handleSetupCronJob — составной скил: добавление задачи в crontab.
// Безопасно добавляет запись, не затирая существующие.
func handleSetupCronJob(args map[string]interface{}) map[string]interface{} {
	schedule, _ := args["schedule"].(string)
	command, _ := args["command"].(string)
	if schedule == "" || command == "" {
		return map[string]interface{}{"error": "schedule и command обязательны"}
	}

	cronEntry := schedule + " " + command
	addCmd := fmt.Sprintf("(crontab -l 2>/dev/null; echo '%s') | sort -u | crontab -", cronEntry)

	result, err := callTool("execute", map[string]interface{}{"command": addCmd})
	if err != nil {
		return map[string]interface{}{"error": "Ошибка добавления в crontab: " + err.Error()}
	}

	// Проверяем что добавилось
	verify, _ := callTool("execute", map[string]interface{}{"command": "crontab -l"})

	return map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Задача добавлена в crontab: %s", cronEntry),
		"result":  result,
		"crontab": verify,
	}
}

// handleProjectInit — составной скил: инициализация нового проекта.
// Создаёт директорию, README.md, .gitignore и инициализирует git.
func handleProjectInit(args map[string]interface{}) map[string]interface{} {
	path, _ := args["path"].(string)
	name, _ := args["name"].(string)
	if path == "" || name == "" {
		return map[string]interface{}{"error": "path и name обязательны"}
	}
	desc, _ := args["description"].(string)
	if desc == "" {
		desc = "Проект " + name
	}

	var steps []map[string]interface{}

	// Создание директории
	r1, _ := callTool("execute", map[string]interface{}{"command": "mkdir -p " + path})
	steps = append(steps, map[string]interface{}{"step": "mkdir", "result": r1})

	// Создание README.md
	readme := fmt.Sprintf("# %s\n\n%s\n\nСоздан: %s\n", name, desc, "$(date)")
	r2, _ := callTool("write", map[string]interface{}{"path": path + "/README.md", "content": readme})
	steps = append(steps, map[string]interface{}{"step": "readme", "result": r2})

	// Создание .gitignore
	gitignore := "*.log\n*.tmp\n*.swp\n.env\nnode_modules/\n__pycache__/\n.DS_Store\n"
	r3, _ := callTool("write", map[string]interface{}{"path": path + "/.gitignore", "content": gitignore})
	steps = append(steps, map[string]interface{}{"step": "gitignore", "result": r3})

	// Инициализация git
	r4, _ := callTool("execute", map[string]interface{}{"command": fmt.Sprintf("cd %s && git init && git config user.email 'admin@openclaw.local' && git config user.name 'OpenClaw Admin' && git add -A && git commit -m 'init: %s'", path, name)})
	steps = append(steps, map[string]interface{}{"step": "git_init", "result": r4})

	return map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Проект '%s' инициализирован в %s (git, README, .gitignore)", name, path),
		"steps":   steps,
	}
}

// ============================================================================
// НОВЫЕ УНИВЕРСАЛЬНЫЕ LEGO-БЛОКИ (обработчики)
// Каждый обработчик выполняет цепочку базовых инструментов за один вызов.
// Умная модель (7B+) предпочтёт базовые инструменты и сама построит цепочку.
// Слабая модель (3B) вызовет один составной скил и получит готовый результат.
// ============================================================================

// handleCheckStack — LEGO-блок: проверка установленных версий программ.
// Для каждой программы из списка выполняет команду определения версии
// и собирает результаты в единый отчёт. Поддерживает: go, node, npm,
// python3, psql, docker, git, nginx, redis-server, curl, wget и любые другие.
func handleCheckStack(args map[string]interface{}) map[string]interface{} {
	programsRaw, ok := args["programs"]
	if !ok {
		return map[string]interface{}{"error": "programs обязателен"}
	}

	var programs []string
	switch v := programsRaw.(type) {
	case []interface{}:
		for _, p := range v {
			if s, ok := p.(string); ok {
				programs = append(programs, s)
			}
		}
	case []string:
		programs = v
	default:
		return map[string]interface{}{"error": "programs должен быть массивом строк"}
	}

	if len(programs) == 0 {
		return map[string]interface{}{"error": "programs пуст"}
	}

	// Маппинг программа → команда для проверки версии.
	// Для известных программ используем специфичную команду,
	// для неизвестных — пробуем --version.
	versionCommands := map[string]string{
		"go":           "go version",
		"node":         "node --version",
		"npm":          "npm --version",
		"python3":      "python3 --version",
		"python":       "python3 --version",
		"psql":         "psql --version",
		"docker":       "docker --version",
		"git":          "git --version",
		"nginx":        "nginx -v 2>&1",
		"redis-server": "redis-server --version",
		"curl":         "curl --version | head -1",
		"wget":         "wget --version | head -1",
		"java":         "java -version 2>&1 | head -1",
		"rustc":        "rustc --version",
		"cargo":        "cargo --version",
		"php":          "php --version | head -1",
		"ruby":         "ruby --version",
		"pip":          "pip3 --version",
		"pip3":         "pip3 --version",
		"gcc":          "gcc --version | head -1",
		"make":         "make --version | head -1",
		"systemctl":    "systemctl --version | head -1",
	}

	var results []map[string]interface{}
	installed := 0
	missing := 0

	for _, prog := range programs {
		cmd, known := versionCommands[prog]
		if !known {
			cmd = prog + " --version 2>&1 | head -1"
		}

		// Проверяем наличие программы через which + версию
		checkCmd := fmt.Sprintf("which %s >/dev/null 2>&1 && %s || echo 'НЕ УСТАНОВЛЕНО'", prog, cmd)
		r, err := callTool("execute", map[string]interface{}{"command": checkCmd})

		entry := map[string]interface{}{
			"program": prog,
		}
		if err != nil {
			entry["status"] = "ошибка"
			entry["error"] = err.Error()
			missing++
		} else {
			output, _ := r["output"].(string)
			if strings.Contains(output, "НЕ УСТАНОВЛЕНО") {
				entry["status"] = "не установлено"
				missing++
			} else {
				entry["status"] = "установлено"
				entry["version"] = strings.TrimSpace(output)
				installed++
			}
		}
		results = append(results, entry)
	}

	return map[string]interface{}{
		"success":   true,
		"message":   fmt.Sprintf("Проверено %d программ: %d установлено, %d отсутствует", len(programs), installed, missing),
		"installed": installed,
		"missing":   missing,
		"programs":  results,
	}
}

// handleDiagnoseService — LEGO-блок: диагностика сервиса.
// Проверяет: 1) занят ли указанный порт, 2) работает ли процесс,
// 3) HTTP-ответ health_url (если указан), 4) последние строки логов.
// Возвращает структурированный отчёт о состоянии сервиса.
func handleDiagnoseService(args map[string]interface{}) map[string]interface{} {
	serviceName, _ := args["service_name"].(string)
	port, _ := args["port"].(float64)
	healthURL, _ := args["health_url"].(string)

	if serviceName == "" || port == 0 {
		return map[string]interface{}{"error": "service_name и port обязательны"}
	}

	report := map[string]interface{}{
		"service": serviceName,
		"port":    int(port),
	}

	// Шаг 1: Проверяем, занят ли порт (кто слушает)
	portCheck, err := callTool("execute", map[string]interface{}{
		"command": fmt.Sprintf("ss -tlnp 2>/dev/null | grep ':%d ' || echo 'порт %d не занят'", int(port), int(port)),
	})
	if err == nil {
		report["port_check"] = portCheck
	}

	// Шаг 2: Проверяем процесс по имени сервиса
	procCheck, err := callTool("execute", map[string]interface{}{
		"command": fmt.Sprintf("pgrep -fa '%s' 2>/dev/null || echo 'процесс %s не найден'", serviceName, serviceName),
	})
	if err == nil {
		report["process_check"] = procCheck
	}

	// Шаг 3: HTTP-проверка здоровья (если указан URL)
	if healthURL != "" {
		healthCheck, err := callTool("execute", map[string]interface{}{
			"command": fmt.Sprintf("curl -s -o /dev/null -w '%%{http_code}' --connect-timeout 3 '%s' 2>/dev/null || echo 'недоступен'", healthURL),
		})
		if err == nil {
			report["health_check"] = healthCheck
		}
	}

	// Шаг 4: Проверяем журнал systemd (если сервис системный)
	journalCheck, err := callTool("execute", map[string]interface{}{
		"command": fmt.Sprintf("journalctl -u %s --no-pager -n 5 2>/dev/null || echo 'журнал systemd недоступен для %s'", serviceName, serviceName),
	})
	if err == nil {
		report["journal"] = journalCheck
	}

	report["success"] = true
	report["message"] = fmt.Sprintf("Диагностика сервиса '%s' (порт %d) завершена", serviceName, int(port))
	return report
}

// handleWebResearch — LEGO-блок: поиск информации в интернете.
// Выполняет internet_search по указанной теме, затем загружает текст
// лучших результатов через browser_get_text. Возвращает сводку.
// Если browser-service недоступен, возвращает только результаты поиска.
func handleWebResearch(args map[string]interface{}) map[string]interface{} {
	topic, _ := args["topic"].(string)
	if topic == "" {
		return map[string]interface{}{"error": "topic обязателен"}
	}

	maxSources := 3
	if m, ok := args["max_sources"].(float64); ok && m > 0 {
		maxSources = int(m)
	}

	report := map[string]interface{}{
		"topic": topic,
	}

	// Шаг 1: Поиск в интернете через browser-service
	searchResult, err := callTool("internet_search", map[string]interface{}{"query": topic})
	if err != nil {
		// Если browser-service недоступен, пробуем через execute + curl
		fallbackResult, fallbackErr := callTool("execute", map[string]interface{}{
			"command": fmt.Sprintf("curl -s 'https://api.duckduckgo.com/?q=%s&format=json&no_html=1' 2>/dev/null | head -c 2000", topic),
		})
		if fallbackErr != nil {
			return map[string]interface{}{"error": "Поиск недоступен: " + err.Error()}
		}
		report["search_fallback"] = fallbackResult
		report["success"] = true
		report["message"] = "Выполнен поиск через DuckDuckGo API (browser-service недоступен)"
		return report
	}

	report["search_results"] = searchResult

	// Шаг 2: Загружаем текст с лучших результатов (если есть URL-ы)
	var sources []map[string]interface{}
	if results, ok := searchResult["results"].([]interface{}); ok {
		limit := maxSources
		if len(results) < limit {
			limit = len(results)
		}
		for i := 0; i < limit; i++ {
			if item, ok := results[i].(map[string]interface{}); ok {
				if url, ok := item["url"].(string); ok && url != "" {
					text, textErr := callTool("browser_get_text", map[string]interface{}{"url": url})
					source := map[string]interface{}{
						"url":   url,
						"title": item["title"],
					}
					if textErr == nil {
						source["content"] = text
					} else {
						source["error"] = textErr.Error()
					}
					sources = append(sources, source)
				}
			}
		}
	}

	if len(sources) > 0 {
		report["sources"] = sources
	}

	report["success"] = true
	report["message"] = fmt.Sprintf("Исследование темы '%s': найдено результатов, загружено %d источников", topic, len(sources))
	return report
}

// handleCheckResourcesBatch — LEGO-блок: проверка доступности нескольких URL.
// Для каждого URL выполняет check_url_access через tools-service.
// Возвращает сводную таблицу доступности всех ресурсов.
func handleCheckResourcesBatch(args map[string]interface{}) map[string]interface{} {
	urlsRaw, ok := args["urls"]
	if !ok {
		return map[string]interface{}{"error": "urls обязателен"}
	}

	var urls []string
	switch v := urlsRaw.(type) {
	case []interface{}:
		for _, u := range v {
			if s, ok := u.(string); ok {
				urls = append(urls, s)
			}
		}
	case []string:
		urls = v
	default:
		return map[string]interface{}{"error": "urls должен быть массивом строк"}
	}

	if len(urls) == 0 {
		return map[string]interface{}{"error": "urls пуст"}
	}

	var results []map[string]interface{}
	accessible := 0
	failed := 0

	for _, url := range urls {
		r, err := callTool("check_url_access", map[string]interface{}{"url": url})
		entry := map[string]interface{}{
			"url": url,
		}
		if err != nil {
			entry["status"] = "ошибка"
			entry["error"] = err.Error()
			failed++
		} else {
			entry["result"] = r
			// Определяем доступность по результату
			if errMsg, hasErr := r["error"]; hasErr && errMsg != nil {
				entry["status"] = "недоступен"
				failed++
			} else {
				entry["status"] = "доступен"
				accessible++
			}
		}
		results = append(results, entry)
	}

	return map[string]interface{}{
		"success":    true,
		"message":    fmt.Sprintf("Проверено %d URL: %d доступно, %d недоступно", len(urls), accessible, failed),
		"accessible": accessible,
		"failed":     failed,
		"results":    results,
	}
}


// handleGenerateReport — LEGO-блок: создание текстового отчёта с верификацией.
// Выполняет: 1) mkdir -p для директории, 2) write содержимого в файл,
// 3) read для проверки записи, 4) stat для проверки размера файла.
// Гарантирует что файл создан и содержит данные.
func handleGenerateReport(args map[string]interface{}) map[string]interface{} {
	path, _ := args["path"].(string)
	content, _ := args["content"].(string)
	title, _ := args["title"].(string)

	if path == "" || content == "" {
		return map[string]interface{}{"error": "path и content обязательны"}
	}

	// Формируем полное содержимое отчёта с заголовком
	fullContent := content
	if title != "" {
		fullContent = fmt.Sprintf("=== %s ===\nДата: $(date)\n\n%s", title, content)
	}

	// Шаг 1: Создаём директорию если нужно
	dir := path[:strings.LastIndex(path, "/")]
	if dir != "" {
		callTool("execute", map[string]interface{}{"command": "mkdir -p " + dir})
	}

	// Шаг 2: Записываем файл
	writeResult, err := callTool("write", map[string]interface{}{"path": path, "content": fullContent})
	if err != nil {
		return map[string]interface{}{"error": "Ошибка записи отчёта: " + err.Error()}
	}

	// Шаг 3: Читаем обратно для верификации
	readResult, err := callTool("read", map[string]interface{}{"path": path})
	if err != nil {
		return map[string]interface{}{
			"success": false,
			"message": "Файл записан, но не удалось прочитать для проверки",
			"write":   writeResult,
			"error":   err.Error(),
		}
	}

	// Шаг 4: Проверяем размер файла
	statResult, _ := callTool("execute", map[string]interface{}{
		"command": fmt.Sprintf("stat -c '%%s байт' '%s' 2>/dev/null || wc -c < '%s'", path, path),
	})

	return map[string]interface{}{
		"success":   true,
		"message":   fmt.Sprintf("Отчёт записан в %s и проверен", path),
		"path":      path,
		"write":     writeResult,
		"verified":  readResult,
		"file_size": statResult,
	}
}

// handleInstallPackages — LEGO-блок: установка пакетов через менеджер пакетов.
// Поддерживает apt, npm, pip. Выполняет установку + проверку версий после.
// Для apt автоматически добавляет sudo и -y флаг.
func handleInstallPackages(args map[string]interface{}) map[string]interface{} {
	packagesRaw, ok := args["packages"]
	if !ok {
		return map[string]interface{}{"error": "packages обязателен"}
	}

	var packages []string
	switch v := packagesRaw.(type) {
	case []interface{}:
		for _, p := range v {
			if s, ok := p.(string); ok {
				packages = append(packages, s)
			}
		}
	case []string:
		packages = v
	default:
		return map[string]interface{}{"error": "packages должен быть массивом строк"}
	}

	if len(packages) == 0 {
		return map[string]interface{}{"error": "packages пуст"}
	}

	manager, _ := args["manager"].(string)
	if manager == "" {
		manager = "apt"
	}

	var installCmd string
	switch manager {
	case "apt":
		installCmd = "sudo apt-get install -y " + strings.Join(packages, " ")
	case "npm":
		installCmd = "npm install -g " + strings.Join(packages, " ")
	case "pip":
		installCmd = "pip3 install " + strings.Join(packages, " ")
	default:
		return map[string]interface{}{"error": fmt.Sprintf("Неизвестный менеджер пакетов: %s (поддерживаются: apt, npm, pip)", manager)}
	}

	// Шаг 1: Обновляем индекс (только для apt)
	var steps []map[string]interface{}
	if manager == "apt" {
		updateResult, _ := callTool("execute", map[string]interface{}{"command": "sudo apt-get update -qq"})
		steps = append(steps, map[string]interface{}{"step": "update_index", "result": updateResult})
	}

	// Шаг 2: Устанавливаем пакеты
	installResult, err := callTool("execute", map[string]interface{}{"command": installCmd})
	if err != nil {
		return map[string]interface{}{
			"error":   "Ошибка установки: " + err.Error(),
			"command": installCmd,
			"steps":   steps,
		}
	}
	steps = append(steps, map[string]interface{}{"step": "install", "command": installCmd, "result": installResult})

	// Шаг 3: Проверяем версии установленных пакетов
	for _, pkg := range packages {
		var verifyCmd string
		switch manager {
		case "apt":
			verifyCmd = fmt.Sprintf("dpkg -l %s 2>/dev/null | tail -1 || echo 'не найден'", pkg)
		case "npm":
			verifyCmd = fmt.Sprintf("npm list -g %s 2>/dev/null | tail -1 || echo 'не найден'", pkg)
		case "pip":
			verifyCmd = fmt.Sprintf("pip3 show %s 2>/dev/null | grep Version || echo 'не найден'", pkg)
		}
		verifyResult, _ := callTool("execute", map[string]interface{}{"command": verifyCmd})
		steps = append(steps, map[string]interface{}{"step": "verify_" + pkg, "result": verifyResult})
	}

	return map[string]interface{}{
		"success":  true,
		"message":  fmt.Sprintf("Установлено %d пакетов через %s", len(packages), manager),
		"manager":  manager,
		"packages": packages,
		"steps":    steps,
	}
}

// initProvidersFromDB — загрузка конфигурации облачных провайдеров из PostgreSQL.
// Вызывается при старте сервиса после инициализации БД.
// Для каждого включённого провайдера регистрирует его в глобальном реестре
// с API-ключом, базовым URL и дополнительными параметрами (folder_id/scope).
// Это позволяет сохранять настройки провайдеров между перезапусками сервиса.
func initProvidersFromDB() {
	var configs []models.ProviderConfig
	db.DB.Where("enabled = ?", true).Find(&configs)
	for _, cfg := range configs {
		extra := cfg.FolderID
		if cfg.Scope != "" {
			extra = cfg.Scope
		}
		if err := llm.RegisterProvider(cfg.ProviderName, cfg.APIKey, cfg.BaseURL, extra, cfg.ServiceAccountJSON); err != nil {
			log.Printf("Failed to register provider %s from DB: %v", cfg.ProviderName, err)
		}
	}
}

// main — точка входа agent-service.
//
// Порядок инициализации:
//  1. Подключение к PostgreSQL и миграции (db.InitDB)
//  2. Инициализация локального провайдера Ollama (llm.InitProviders)
//  3. Загрузка конфигурации облачных провайдеров из БД (initProvidersFromDB)
//  4. Создание агента Admin по умолчанию, если его нет
//  5. Инициализация метрик OpenTelemetry
//  6. Регистрация HTTP-обработчиков для всех эндпоинтов
//  7. Настройка раздачи статических файлов из uploads/
//  8. Запуск HTTP-сервера на порту AGENT_SERVICE_PORT (по умолчанию 8083)
func validateEnv() {
	log.Println("=== Проверка переменных окружения ===")

	dbURL := os.Getenv("DATABASE_URL")
	dbHost := os.Getenv("DB_HOST")
	if dbURL == "" && dbHost == "" {
		log.Println("[ENV] DATABASE_URL и DB_HOST не заданы — будут использованы значения по умолчанию (localhost:5432)")
		log.Println("[ENV] Для настройки см. .env.example или документацию")
	}

	port := getEnv("AGENT_SERVICE_PORT", "8083")
	log.Printf("[ENV] Порт agent-service: %s", port)

	toolsURL := getEnv("TOOLS_SERVICE_URL", "http://localhost:8082")
	memoryURL := getEnv("MEMORY_SERVICE_URL", "http://localhost:8001")
	log.Printf("[ENV] tools-service: %s", toolsURL)
	log.Printf("[ENV] memory-service: %s", memoryURL)

	ollamaURL := getEnv("OLLAMA_URL", "")
	if ollamaURL == "" {
		ollamaURL = getEnv("OLLAMA_HOST", "http://localhost:11434")
	}
	log.Printf("[ENV] Ollama: %s", ollamaURL)

	log.Println("=== Проверка окружения завершена ===")
}

func main() {
	validateEnv()

	db.InitDB()

	llm.InitProviders()
	initProvidersFromDB()
	initRAG()

	// Инициализация метрик Prometheus
	metrics.Init()
	log.Printf("[METRICS] Метрики инициализированы")

	if err := repository.CreateDefaultAgents(); err != nil {
		log.Fatalf("Failed to create default agents: %v", err)
	}

	// Регистрация метрик endpoint (должна быть перед catch-all роутером)
	http.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		h := metrics.InitPrometheusHandler()
		h.ServeHTTP(w, r)
	})

	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/chat", chatHandler)
	http.HandleFunc("/agents", agentsHandler)
	http.HandleFunc("/models", modelsHandler)
	http.HandleFunc("/prompts", promptsHandler)
	http.HandleFunc("/prompts/load", loadPromptHandler)
	http.HandleFunc("/agent/prompt", updatePromptHandler)
	http.HandleFunc("/update-model", updateAgentModelHandler)
	http.HandleFunc("/avatar", avatarUploadHandler)
	http.HandleFunc("/avatar-info", avatarGetHandler)
	http.HandleFunc("/providers", providersHandler)
	http.HandleFunc("/cloud-models", cloudModelsHandler)
	http.HandleFunc("/workspaces", workspacesHandler)
	http.HandleFunc("/learning-stats", learningStatsHandler)
	http.HandleFunc("/logs", logsHandler)

	// RAG эндпоинты
	http.HandleFunc("/rag/add", ragAddHandler)
	http.HandleFunc("/rag/add-folder", ragAddFolderHandler)
	http.HandleFunc("/rag/search", ragSearchHandler)
	http.HandleFunc("/rag/files", ragFilesHandler)
	http.HandleFunc("/rag/stats", ragStatsHandler)
	http.HandleFunc("/rag/delete", ragDeleteHandler)

	for _, dir := range []string{
		filepath.Join(".", "uploads"),
		filepath.Join(".", "uploads", "avatars"),
		filepath.Join(".", "prompts"),
		filepath.Join(".", "prompts", "admin"),
	} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			log.Printf("Warning: failed to create directory %s: %v", dir, err)
		}
	}

	uploadDir := filepath.Join(".", "uploads")
	http.Handle("/uploads/", http.StripPrefix("/uploads/", http.FileServer(http.Dir(uploadDir))))

	http.HandleFunc("/", rootHandler)

	port := getEnv("AGENT_SERVICE_PORT", "8083")

	log.Printf("Agent service starting on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
