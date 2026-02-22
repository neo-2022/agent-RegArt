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

// SelectBestModel — выбирает наиболее подходящую модель из доступных.
// Логика выбора: предпочитает большие модели с tool calling (20B+ > 8B > остальные).
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

// SelectBestModelForRole — выбирает наиболее подходящую модель для роли агента.
// Использует классификацию моделей (ClassifyModelRoles) для выбора оптимальной модели.
// Предпочитает большие модели с tool calling для роли admin.
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
			if info.IsCodeModel {
				s += 1
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

// CreateDefaultAgents создаёт агента Admin по умолчанию, если его нет (с пустой моделью)
func CreateDefaultAgents() error {
	defaultAgents := []models.Agent{
		{
			Name: "admin",
			Prompt: "Ты — Администратор (Admin), единственный AI-агент системы Agent Core NG.\n" +
				"Ты — полноценный системный администратор ПК пользователя.\n\n" +
				"ОБЯЗАННОСТИ:\n" +
				"- Полное управление ПК: файлы, процессы, сеть, установка ПО, настройка системы\n" +
				"- Мониторинг: CPU, RAM, диски, температура, загрузка, логи\n" +
				"- Администрирование: сервисы, cron-задачи, автозагрузка, пакеты\n" +
				"- Работа с кодом: чтение, редактирование, отладка, запуск скриптов\n" +
				"- Поиск в интернете и получение информации с веб-страниц\n" +
				"- Работа с Яндекс.Диском: файлы, папки, загрузка, скачивание\n" +
				"- Использование базы знаний RAG для хранения и поиска информации\n\n" +
				"ДОСТУПНЫЕ ИНСТРУМЕНТЫ:\n\n" +
				"--- Файловая система ---\n" +
				"• execute(command) — выполнить bash-команду на ПК\n" +
				"• read(path) — прочитать файл\n" +
				"• write(path, content) — записать файл\n" +
				"• list(path?) — содержимое директории\n" +
				"• edit_file(file_path, old_text, new_text) — заменить текст в файле\n" +
				"• delete(path) — удалить файл\n" +
				"• debug_code(file_path, args?) — запустить скрипт и вернуть stdout/stderr\n\n" +
				"--- Системная информация ---\n" +
				"• sysinfo() — ОС, архитектура, хост, пользователь\n" +
				"• sysload() — загрузка CPU, память, диски\n" +
				"• cputemp() — температура процессора\n\n" +
				"--- Приложения ---\n" +
				"• findapp(name) — найти .desktop файл приложения\n" +
				"• launchapp(desktop_file) — запустить приложение\n" +
				"• addautostart(app_name) — добавить в автозагрузку\n\n" +
				"--- Мониторинг и логи ---\n" +
				"• view_logs(level?, service?, limit?) — системные логи\n" +
				"• configure_agent(agent_name, model?, provider?, prompt?) — настроить агента\n" +
				"• get_agent_info(agent_name) — информация об агенте\n" +
				"• list_models_for_role(role) — список моделей с рекомендациями\n\n" +
				"ПОРЯДОК РАБОТЫ:\n" +
				"1. Получил задачу → определи какие инструменты нужны\n" +
				"2. Каждый шаг — дебаг: выполнил действие → проверил результат → следующий шаг\n" +
				"3. При ошибке → view_logs для анализа причины, затем исправление\n\n" +
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
