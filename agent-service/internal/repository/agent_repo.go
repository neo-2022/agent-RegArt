package repository

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/neo-2022/openclaw-memory/agent-service/internal/db"
	"github.com/neo-2022/openclaw-memory/agent-service/internal/models"
	"gorm.io/gorm"
)

func getOllamaAPIURL() string {
	if url := os.Getenv("OLLAMA_URL"); url != "" {
		return strings.TrimRight(url, "/")
	}
	if url := os.Getenv("OLLAMA_HOST"); url != "" {
		return strings.TrimRight(url, "/")
	}
	return "http://localhost:11434"
}

// Список моделей с поддержкой инструментов определяется динамически
// через CheckModelToolSupport() — никаких жёстких привязок в коде.

// GetOllamaModels возвращает список доступных моделей из локальной Ollama
func GetOllamaModels() ([]string, error) {
	// Пытаемся использовать ollama list (быстрее, но требует наличия ollama в PATH)
	cmd := exec.Command("ollama", "list")
	out, err := cmd.Output()
	if err == nil {
		// Парсим вывод: первая строка заголовок, потом NAME ID SIZE MODIFIED
		lines := strings.Split(string(out), "\n")
		var models []string
		for i, line := range lines {
			if i == 0 || strings.TrimSpace(line) == "" {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) > 0 {
				models = append(models, fields[0])
			}
		}
		if len(models) > 0 {
			return models, nil
		}
	}

	// Если ollama list не сработал, пробуем через API
	resp, err := http.Get(getOllamaAPIURL() + "/api/tags")
	if err != nil {
		return nil, fmt.Errorf("cannot connect to Ollama: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse Ollama response: %w", err)
	}

	var models []string
	for _, m := range result.Models {
		models = append(models, m.Name)
	}
	return models, nil
}

// SelectBestModel — выбирает наиболее подходящую модель из доступных для конкретной роли.
// Логика выбора:
//   - Для admin: предпочитает большие модели с tool calling (20B+ > 8B > остальные)
//   - Для coder: предпочитает кодовые модели с tool calling (qwen2.5-coder > общие)
//   - Для novice: любая модель (предпочитает универсальные средних размеров)
//
// Если требуется поддержка инструментов (needTools=true), проверяет каждую модель
// через кэш в БД (GetModelToolSupport). Никаких жёстких списков —
// пригодность определяется динамически через тестовый вызов.
func SelectBestModel(available []string, needTools bool) (string, error) {
	if len(available) == 0 {
		return "", errors.New("нет доступных моделей в Ollama")
	}
	if needTools {
		var toolModels []string
		for _, avail := range available {
			supports, err := GetModelToolSupport(avail)
			if err == nil && supports {
				toolModels = append(toolModels, avail)
			}
		}
		if len(toolModels) > 0 {
			return toolModels[0], nil
		}
		return available[0], nil
	}
	return available[0], nil
}

// SelectBestModelForRole — выбирает наиболее подходящую модель для конкретной роли агента.
// Использует классификацию моделей (ClassifyModelRoles) для выбора оптимальной модели:
//   - admin: предпочитает большие модели с tool calling
//   - coder: предпочитает кодовые модели с tool calling
//   - novice: любая модель, предпочитает универсальные
func SelectBestModelForRole(available []string, role string) (string, error) {
	if len(available) == 0 {
		return "", errors.New("нет доступных моделей в Ollama")
	}

	type scoredModel struct {
		name  string
		score int
	}
	var scored []scoredModel

	for _, m := range available {
		info, err := GetModelFullInfo(m)
		if err != nil {
			scored = append(scored, scoredModel{name: m, score: 0})
			continue
		}

		s := 0
		var roles []string
		json.Unmarshal([]byte(info.SuitableRoles), &roles)
		for _, r := range roles {
			if r == role {
				s += 10
				break
			}
		}

		if info.SupportsTools {
			s += 5
		}

		paramSize := parseParamSize(info.ParameterSize)

		switch role {
		case "admin":
			if paramSize >= 20 {
				s += 4
			} else if paramSize >= 8 {
				s += 2
			}
		case "coder":
			if info.IsCodeModel {
				s += 6
			}
			if paramSize >= 7 {
				s += 2
			}
		case "novice":
			if paramSize >= 7 && paramSize <= 10 {
				s += 3
			}
		}

		scored = append(scored, scoredModel{name: m, score: s})
	}

	best := scored[0]
	for _, sm := range scored[1:] {
		if sm.score > best.score {
			best = sm
		}
	}
	return best.name, nil
}

// EnsureAgentModel проверяет, что у агента указана существующая модель, и при необходимости обновляет
func EnsureAgentModel(agent *models.Agent) error {
	switch agent.Provider {
	case "ollama":
		return ensureOllamaModel(agent)
	default:
		if agent.LLMModel != "" {
			return nil
		}
		return nil
	}
}

func ensureOllamaModel(agent *models.Agent) error {
	// Получаем доступные модели из Ollama
	available, err := GetOllamaModels()
	if err != nil {
		return fmt.Errorf("cannot get available models from Ollama: %w", err)
	}
	if len(available) == 0 {
		return errors.New("no models installed in Ollama. Please install at least one model, e.g., 'ollama pull qwen2.5-coder'")
	}

	// Если у агента уже есть модель, проверяем её наличие
	if agent.LLMModel != "" {
		for _, m := range available {
			if m == agent.LLMModel {
				return nil // модель существует
			}
		}
		// Модель не найдена, будем выбирать новую
	}

	// Выбираем подходящую модель
	best, err := SelectBestModel(available, agent.SupportsTools)
	if err != nil {
		return err
	}

	// Обновляем модель агента
	agent.LLMModel = best
	if err := db.DB.Save(agent).Error; err != nil {
		return fmt.Errorf("failed to update agent model: %w", err)
	}
	return nil
}

// GetAgentByName возвращает агента по имени, предварительно проверяя модель
func GetAgentByName(name string) (*models.Agent, error) {
	var agent models.Agent
	err := db.DB.Where("name = ?", name).First(&agent).Error
	if err != nil {
		return nil, err
	}

	// Проверяем и обновляем модель при необходимости
	if err := EnsureAgentModel(&agent); err != nil {
		return nil, err
	}
	return &agent, nil
}

// CreateDefaultAgents создаёт агентов по умолчанию, если их нет (с пустыми моделями)
func CreateDefaultAgents() error {
	defaultAgents := []models.Agent{
		{
			Name: "admin",
			Prompt: "Ты — Администратор (Admin), главный агент системы Agent Core NG.\n\n" +
				"ОБЯЗАННОСТИ:\n" +
				"- Полное управление ПК пользователя: файлы, процессы, сеть, установка ПО\n" +
				"- Делегирование задач Кодеру и Послушнику\n" +
				"- Контроль работы подчинённых агентов, дебаг их результатов\n" +
				"- Настройка агентов: подбор моделей, промптов, провайдеров\n" +
				"- Мониторинг системных логов и исправление ошибок\n\n" +
				"ДОСТУПНЫЕ КОМАНДЫ (инструменты):\n\n" +
				"--- Управление агентами (только Admin) ---\n" +
				"• call_coder(task) — делегировать задачу Кодеру (программирование, анализ кода)\n" +
				"• call_novice(task) — делегировать задачу Послушнику (поиск информации, простые задачи)\n" +
				"• configure_agent(agent_name, model?, provider?, prompt?) — настроить агента: сменить модель, провайдер или промпт\n" +
				"• get_agent_info(agent_name) — получить текущие настройки агента (модель, провайдер, промпт, поддержка инструментов)\n" +
				"• list_models_for_role(role) — список моделей с рекомендациями для роли (admin/coder/novice)\n\n" +
				"--- Мониторинг и логи (только Admin) ---\n" +
				"• view_logs(level?, service?, limit?) — просмотр системных логов. Фильтры: level=error/warn/info, service=agent-service/tools-service/memory-service/api-gateway\n\n" +
				"--- Файловая система ---\n" +
				"• execute(command) — выполнить bash-команду на ПК пользователя\n" +
				"• read(path) — прочитать содержимое файла\n" +
				"• write(path, content) — записать содержимое в файл\n" +
				"• list(path?) — показать содержимое директории (по умолчанию текущая)\n" +
				"• edit_file(file_path, old_text, new_text) — заменить текст в файле\n" +
				"• delete(path) — удалить файл\n\n" +
				"--- Отладка ---\n" +
				"• debug_code(file_path, args?) — запустить скрипт/код и вернуть stdout/stderr\n\n" +
				"--- Системная информация ---\n" +
				"• sysinfo() — информация о системе (ОС, хост, архитектура)\n" +
				"• sysload() — загрузка системы (CPU, память, диски)\n" +
				"• cputemp() — температура процессора\n\n" +
				"--- Приложения ---\n" +
				"• findapp(name) — найти .desktop файл приложения по имени\n" +
				"• launchapp(desktop_file) — запустить приложение по .desktop файлу\n" +
				"• addautostart(app_name) — добавить приложение в автозагрузку\n\n" +
				"ПОРЯДОК РАБОТЫ:\n" +
				"1. Получил задачу → определи какие инструменты нужны\n" +
				"2. Если задача по коду → call_coder. Если простой поиск → call_novice\n" +
				"3. Каждый шаг — дебаг: выполнил действие → проверил результат → следующий шаг\n" +
				"4. При ошибке → view_logs для анализа причины, затем исправление\n" +
				"5. При настройке агентов → list_models_for_role → configure_agent\n\n" +
				"ПРАВИЛА:\n" +
				"1. НИКОГДА не додумывай информацию. Используй только данные из инструментов.\n" +
				"2. Каждый шаг — дебаг. Выполнил действие → проверил результат → следующий шаг.\n" +
				"3. Если не можешь выполнить задачу — честно скажи и предложи варианты.\n" +
				"4. Всегда используй инструменты для получения реальных данных.\n" +
				"5. При ошибке — анализируй причину через view_logs, не повторяй одно и то же.\n" +
				"6. Отвечай на русском языке.",
			LLMModel:      "",
			Provider:      "ollama",
			SupportsTools: true,
		},
		{
			Name: "coder",
			Prompt: "Ты — Кодер (Coder), агент для написания и анализа кода.\n\n" +
				"ОБЯЗАННОСТИ:\n" +
				"- Написание, редактирование и анализ кода\n" +
				"- Работа с файлами проекта через инструменты\n" +
				"- Отладка и исправление ошибок в коде\n\n" +
				"ПРАВИЛА:\n" +
				"1. НИКОГДА не додумывай структуру проекта — сначала прочитай файлы через инструменты.\n" +
				"2. Каждый шаг — дебаг. Написал код → проверил → исправил если нужно.\n" +
				"3. Если не знаешь ответ или не можешь выполнить задачу — спроси у Админа.\n" +
				"4. Не выполняй системные команды (перезагрузка, установка ПО) — это задача Админа.\n" +
				"5. Используй только реальные данные из файлов, не генерируй выдуманный код.",
			LLMModel:      "",
			Provider:      "ollama",
			SupportsTools: true,
		},
		{
			Name: "novice",
			Prompt: "Ты — Послушник (Novice), помощник для второстепенных задач.\n\n" +
				"ОБЯЗАННОСТИ:\n" +
				"- Поиск информации и ответы на вопросы\n" +
				"- Помощь с простыми задачами (перевод, форматирование, советы)\n" +
				"- Выполнение поручений от Админа\n\n" +
				"ПРАВИЛА:\n" +
				"1. НИКОГДА не сочиняй информацию — если не знаешь, скажи честно.\n" +
				"2. Если задача сложная или требует системных команд — передай Админу.\n" +
				"3. Не выполняй действия с файлами и системой — у тебя нет таких инструментов.\n" +
				"4. При неуверенности — спроси у Админа или пользователя.\n" +
				"5. Давай только проверенные ответы, основанные на фактах.",
			LLMModel:      "",
			Provider:      "ollama",
			SupportsTools: false,
		},
	}

	for _, a := range defaultAgents {
		var existing models.Agent
		err := db.DB.Where("name = ?", a.Name).First(&existing).Error
		if err != nil && errors.Is(err, gorm.ErrRecordNotFound) {
			if err := db.DB.Create(&a).Error; err != nil {
				return err
			}
		}
	}
	return nil
}
