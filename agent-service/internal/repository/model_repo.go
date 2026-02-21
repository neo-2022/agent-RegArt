// Пакет repository — слой доступа к данным для agent-service.
// model_repo.go — управление кэшем моделей, автоматическое определение возможностей
// и классификация моделей по подходящим ролям агентов (admin, coder, novice).
//
// Вся информация о моделях получается динамически:
//   - Список моделей — из Ollama API /api/tags (или ollama list)
//   - Метаданные (семейство, размер) — из Ollama API /api/show
//   - Поддержка инструментов — тестовый запрос к модели
//   - Классификация ролей — автоматически на основе метаданных
//
// Никаких жёстких привязок моделей в коде нет. Всё определяется автоматически.
package repository

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/neo-2022/openclaw-memory/agent-service/internal/db"
	"github.com/neo-2022/openclaw-memory/agent-service/internal/models"
)

func getOllamaBaseURL() string {
	if url := os.Getenv("OLLAMA_URL"); url != "" {
		return strings.TrimRight(url, "/")
	}
	if url := os.Getenv("OLLAMA_HOST"); url != "" {
		return strings.TrimRight(url, "/")
	}
	return "http://localhost:11434"
}

// OllamaModelDetails — метаданные модели, полученные из Ollama API /api/show.
// Содержит информацию о семействе модели, размере параметров и уровне квантования.
// Эти данные используются для автоматической классификации модели по ролям.
type OllamaModelDetails struct {
	Family        string `json:"family"`
	ParameterSize string `json:"parameter_size"`
	Quantization  string `json:"quantization_level"`
}

// ModelRoleInfo — результат автоматической классификации модели по ролям агентов.
// Содержит информацию о подходящих ролях и пояснения для каждой роли.
type ModelRoleInfo struct {
	SuitableRoles []string          `json:"suitable_roles"`
	RoleNotes     map[string]string `json:"role_notes"`
}

// GetModelDetails — получает метаданные модели из Ollama API /api/show.
// Возвращает семейство модели, размер параметров и уровень квантования.
// Если Ollama недоступна или модель не найдена — возвращает пустую структуру без ошибки.
func GetModelDetails(modelName string) OllamaModelDetails {
	reqBody, _ := json.Marshal(map[string]string{"name": modelName})
	resp, err := http.Post(getOllamaBaseURL()+"/api/show", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		log.Printf("Не удалось получить метаданные модели %s: %v", modelName, err)
		return OllamaModelDetails{}
	}
	defer resp.Body.Close()

	var result struct {
		Details OllamaModelDetails `json:"details"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Printf("Ошибка парсинга метаданных модели %s: %v", modelName, err)
		return OllamaModelDetails{}
	}
	return result.Details
}

// parseParamSize — извлекает числовое значение размера модели из строки (например, "8B" → 8.0).
// Поддерживает суффиксы B (миллиарды) и M (миллионы, конвертируются в миллиарды).
func parseParamSize(s string) float64 {
	s = strings.TrimSpace(strings.ToUpper(s))
	if s == "" {
		return 0
	}
	multiplier := 1.0
	if strings.HasSuffix(s, "B") {
		s = strings.TrimSuffix(s, "B")
	} else if strings.HasSuffix(s, "M") {
		s = strings.TrimSuffix(s, "M")
		multiplier = 0.001
	}
	val, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return val * multiplier
}

// isCodeModel — определяет, является ли модель специализированной на генерации кода.
// Проверяет имя модели и семейство на наличие ключевых слов: coder, code, codestral, deepseek-coder.
func isCodeModel(modelName, family string) bool {
	lower := strings.ToLower(modelName)
	codeKeywords := []string{"coder", "code", "codestral", "deepseek-coder", "starcoder", "codellama", "codegemma"}
	for _, kw := range codeKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	familyLower := strings.ToLower(family)
	for _, kw := range codeKeywords {
		if strings.Contains(familyLower, kw) {
			return true
		}
	}
	return false
}

// ClassifyModelRoles — автоматически определяет подходящие роли агентов для модели.
// Классификация основана на:
//   - Поддержке инструментов (tool calling) — обязательно для admin и coder
//   - Специализации на коде — приоритет для роли coder
//   - Размере модели — крупные модели (13B+) лучше для admin
//   - Семействе модели — общие модели подходят для admin, кодовые для coder
//
// Правила классификации:
//   - admin: нужна поддержка инструментов + достаточный размер (7B+) для сложных задач
//   - coder: нужна поддержка инструментов + специализация на коде (или крупная общая модель)
//   - novice: любая модель подходит (не требует инструментов)
func ClassifyModelRoles(modelName string, supportsTools bool, details OllamaModelDetails) ModelRoleInfo {
	info := ModelRoleInfo{
		SuitableRoles: []string{},
		RoleNotes:     make(map[string]string),
	}

	paramSize := parseParamSize(details.ParameterSize)
	isCode := isCodeModel(modelName, details.Family)

	if supportsTools {
		if paramSize >= 7 || paramSize == 0 {
			info.SuitableRoles = append(info.SuitableRoles, "admin")
			if paramSize >= 13 {
				info.RoleNotes["admin"] = "Отлично подходит: большая модель с поддержкой инструментов"
			} else {
				info.RoleNotes["admin"] = "Подходит: поддерживает инструменты для управления системой"
			}
		} else {
			info.RoleNotes["admin"] = "Не рекомендуется: слишком маленькая модель для сложных задач администрирования"
		}

		if isCode {
			info.SuitableRoles = append(info.SuitableRoles, "coder")
			info.RoleNotes["coder"] = "Отлично подходит: модель специализирована на коде + поддерживает инструменты"
		} else if paramSize >= 7 || paramSize == 0 {
			info.SuitableRoles = append(info.SuitableRoles, "coder")
			info.RoleNotes["coder"] = "Подходит: общая модель с поддержкой инструментов (для кода лучше специализированная)"
		} else {
			info.RoleNotes["coder"] = "Не рекомендуется: маленькая общая модель без специализации на коде"
		}
	} else {
		info.RoleNotes["admin"] = "Не подходит: модель не поддерживает вызов инструментов (tool calling)"
		if isCode {
			info.RoleNotes["coder"] = "Ограниченно: модель для кода, но без поддержки инструментов"
		} else {
			info.RoleNotes["coder"] = "Не подходит: нет поддержки инструментов и специализации на коде"
		}
	}

	info.SuitableRoles = append(info.SuitableRoles, "novice")
	if paramSize >= 7 || paramSize == 0 {
		info.RoleNotes["novice"] = "Подходит: может выполнять второстепенные задачи"
	} else {
		info.RoleNotes["novice"] = "Отлично подходит: компактная модель для простых задач"
	}

	return info
}

// CheckModelToolSupport — выполняет тестовый вызов инструмента для модели.
// Отправляет запрос к Ollama с тестовым инструментом и проверяет,
// ответила ли модель вызовом инструмента (tool_calls).
// Это единственный надёжный способ определить поддержку инструментов —
// метаданные модели не содержат этой информации.
func CheckModelToolSupport(modelName string) (bool, error) {
	testTool := map[string]interface{}{
		"type": "function",
		"function": map[string]interface{}{
			"name":        "test_tool",
			"description": "Тестовый инструмент для проверки поддержки tool calling",
			"parameters": map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
	}
	request := map[string]interface{}{
		"model": modelName,
		"messages": []map[string]string{
			{"role": "user", "content": "Вызови тестовый инструмент."},
		},
		"tools":  []interface{}{testTool},
		"stream": false,
	}
	data, err := json.Marshal(request)
	if err != nil {
		return false, err
	}
	resp, err := http.Post(getOllamaBaseURL()+"/api/chat", "application/json", bytes.NewReader(data))
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	var result struct {
		Message struct {
			ToolCalls []interface{} `json:"tool_calls"`
		} `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, err
	}

	return len(result.Message.ToolCalls) > 0, nil
}

// GetModelToolSupport — возвращает полную информацию о модели из кэша.
// Если записи нет — выполняет проверку tool support, получает метаданные,
// классифицирует модель и сохраняет результат в БД.
func GetModelToolSupport(modelName string) (bool, error) {
	var record models.ModelToolSupport
	err := db.DB.Where("model_name = ?", modelName).First(&record).Error
	if err == nil {
		return record.SupportsTools, nil
	}

	supports, err := CheckModelToolSupport(modelName)
	if err != nil {
		return false, err
	}

	details := GetModelDetails(modelName)
	roleInfo := ClassifyModelRoles(modelName, supports, details)
	rolesJSON, _ := json.Marshal(roleInfo.SuitableRoles)
	notesJSON, _ := json.Marshal(roleInfo.RoleNotes)

	record = models.ModelToolSupport{
		ModelName:     modelName,
		SupportsTools: supports,
		Family:        details.Family,
		ParameterSize: details.ParameterSize,
		IsCodeModel:   isCodeModel(modelName, details.Family),
		SuitableRoles: string(rolesJSON),
		RoleNotes:     string(notesJSON),
		CheckedAt:     time.Now(),
	}
	if err := db.DB.Create(&record).Error; err != nil {
		return false, err
	}
	return supports, nil
}

// GetModelFullInfo — возвращает полную запись ModelToolSupport из кэша.
// Если записи нет — выполняет полную классификацию и сохраняет.
// Используется в modelsHandler для возврата всей информации о модели клиенту.
func GetModelFullInfo(modelName string) (*models.ModelToolSupport, error) {
	var record models.ModelToolSupport
	err := db.DB.Where("model_name = ?", modelName).First(&record).Error
	if err == nil {
		return &record, nil
	}

	supports, checkErr := CheckModelToolSupport(modelName)
	if checkErr != nil {
		supports = false
	}

	details := GetModelDetails(modelName)
	roleInfo := ClassifyModelRoles(modelName, supports, details)
	rolesJSON, _ := json.Marshal(roleInfo.SuitableRoles)
	notesJSON, _ := json.Marshal(roleInfo.RoleNotes)

	record = models.ModelToolSupport{
		ModelName:     modelName,
		SupportsTools: supports,
		Family:        details.Family,
		ParameterSize: details.ParameterSize,
		IsCodeModel:   isCodeModel(modelName, details.Family),
		SuitableRoles: string(rolesJSON),
		RoleNotes:     string(notesJSON),
		CheckedAt:     time.Now(),
	}
	db.DB.Create(&record)
	return &record, nil
}

// SyncModels — синхронизирует кэш моделей с текущим списком из Ollama.
// Для новых моделей — выполняет полную классификацию (tool support + метаданные + роли).
// Для удалённых моделей — удаляет записи из кэша.
func SyncModels(ollamaModels []string) error {
	var existing []models.ModelToolSupport
	db.DB.Find(&existing)

	existingMap := make(map[string]models.ModelToolSupport)
	for _, rec := range existing {
		existingMap[rec.ModelName] = rec
	}

	for _, model := range ollamaModels {
		if _, ok := existingMap[model]; !ok {
			supports, err := CheckModelToolSupport(model)
			if err != nil {
				supports = false
			}

			details := GetModelDetails(model)
			roleInfo := ClassifyModelRoles(model, supports, details)
			rolesJSON, _ := json.Marshal(roleInfo.SuitableRoles)
			notesJSON, _ := json.Marshal(roleInfo.RoleNotes)

			newRec := models.ModelToolSupport{
				ModelName:     model,
				SupportsTools: supports,
				Family:        details.Family,
				ParameterSize: details.ParameterSize,
				IsCodeModel:   isCodeModel(model, details.Family),
				SuitableRoles: string(rolesJSON),
				RoleNotes:     string(notesJSON),
				CheckedAt:     time.Now(),
			}
			db.DB.Create(&newRec)
		}
	}

	ollamaSet := make(map[string]bool)
	for _, m := range ollamaModels {
		ollamaSet[m] = true
	}
	for _, rec := range existing {
		if !ollamaSet[rec.ModelName] {
			db.DB.Delete(&rec)
		}
	}
	return nil
}
