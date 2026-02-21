// Пакет skills — система навыков агентов.
//
// Позволяет загружать YAML-файлы с описанием навыков (скилов),
// которые агенты могут использовать для выполнения задач.
// Каждый навык описывает: название, параметры, эндпоинт для вызова,
// шаблон запроса и список агентов, которым он доступен.
//
// Навыки хранятся в директории skills/ в корне проекта.
// При запуске системы все навыки загружаются и становятся доступны
// агентам в зависимости от их роли (admin, coder, novice).
package skills

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// SkillParameter — описание параметра навыка.
// Определяет имя, тип, обязательность, значение по умолчанию и описание.
type SkillParameter struct {
	// Имя параметра (например, "query", "url", "mode")
	Name string `json:"name"`
	// Тип параметра: string, number, boolean
	Type string `json:"type"`
	// Обязательный ли параметр
	Required bool `json:"required"`
	// Значение по умолчанию (если не указано пользователем)
	Default interface{} `json:"default,omitempty"`
	// Подробное описание параметра на русском
	Description string `json:"description"`
}

// Skill — описание навыка агента, загруженное из YAML-файла.
// Содержит всю информацию, необходимую для вызова эндпоинта browser-service
// или другого микросервиса с правильными параметрами.
type Skill struct {
	// Уникальное имя навыка (например, "web_search", "screenshot")
	Name string `json:"name"`
	// Подробное описание на русском — что делает навык
	Description string `json:"description"`
	// Версия навыка
	Version string `json:"version"`
	// Автор навыка
	Author string `json:"author"`
	// Параметры навыка — список входных данных
	Parameters []SkillParameter `json:"parameters"`
	// URL эндпоинта для вызова (например, "http://localhost:8084/search")
	Endpoint string `json:"endpoint"`
	// HTTP-метод: POST, GET, PUT, DELETE
	Method string `json:"method"`
	// Шаблон JSON-запроса с плейсхолдерами {{param_name}}
	Template string `json:"template"`
	// Теги для поиска и группировки навыков
	Tags []string `json:"tags"`
	// Список агентов, которым доступен навык: admin, coder, novice
	Agents []string `json:"agents"`
}

// SkillExecutionResult — результат выполнения навыка.
// Содержит статус, тело ответа и возможную ошибку.
type SkillExecutionResult struct {
	// Успешно ли выполнен навык
	Success bool `json:"success"`
	// HTTP-статус код ответа
	StatusCode int `json:"status_code"`
	// Тело ответа от сервиса
	Body string `json:"body"`
	// Описание ошибки (если есть)
	Error string `json:"error,omitempty"`
}

// SkillLoader — загрузчик и менеджер навыков.
// Загружает YAML-файлы из указанной директории,
// хранит их в памяти и предоставляет методы для поиска и вызова.
type SkillLoader struct {
	// Директория с YAML-файлами навыков
	skillsDir string
	// Кэш загруженных навыков: имя → навык
	skills map[string]*Skill
	// Мьютекс для потокобезопасного доступа к кэшу
	mu sync.RWMutex
	// HTTP-клиент для вызова эндпоинтов навыков
	httpClient *http.Client
}

// NewSkillLoader — создаёт новый загрузчик навыков.
// skillsDir — путь к директории с YAML-файлами навыков.
// Автоматически загружает все навыки из директории при создании.
func NewSkillLoader(skillsDir string) (*SkillLoader, error) {
	loader := &SkillLoader{
		skillsDir: skillsDir,
		skills:    make(map[string]*Skill),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}

	// Загружаем все навыки из директории
	if err := loader.LoadAll(); err != nil {
		return nil, fmt.Errorf("ошибка загрузки навыков из %s: %w", skillsDir, err)
	}

	return loader, nil
}

// LoadAll — загружает все YAML-файлы из директории навыков.
// Сканирует директорию, находит все .yaml и .yml файлы,
// парсит их и добавляет в кэш.
func (l *SkillLoader) LoadAll() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Проверяем существование директории
	if _, err := os.Stat(l.skillsDir); os.IsNotExist(err) {
		return fmt.Errorf("директория навыков не найдена: %s", l.skillsDir)
	}

	// Ищем все YAML-файлы
	entries, err := os.ReadDir(l.skillsDir)
	if err != nil {
		return fmt.Errorf("ошибка чтения директории %s: %w", l.skillsDir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".yaml" && ext != ".yml" {
			continue
		}
		filePath := filepath.Join(l.skillsDir, entry.Name())
		skill, err := l.parseSkillFile(filePath)
		if err != nil {
			// Логируем ошибку, но продолжаем загрузку остальных
			fmt.Printf("[skills] Ошибка парсинга %s: %v\n", filePath, err)
			continue
		}
		l.skills[skill.Name] = skill
	}

	fmt.Printf("[skills] Загружено навыков: %d из директории %s\n", len(l.skills), l.skillsDir)
	return nil
}

// parseSkillFile — парсит YAML-файл навыка в структуру Skill.
// Использует простой парсер (без внешних зависимостей),
// который извлекает поля из YAML построчно.
func (l *SkillLoader) parseSkillFile(filePath string) (*Skill, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения файла %s: %w", filePath, err)
	}

	skill := &Skill{}
	lines := strings.Split(string(data), "\n")

	var currentSection string
	var templateLines []string
	var inTemplate bool
	var currentParam *SkillParameter

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Пропускаем пустые строки и комментарии
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Обработка многострочного шаблона (template)
		if inTemplate {
			if !strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "\t") && strings.Contains(line, ":") && !strings.Contains(line, "{{") {
				inTemplate = false
				skill.Template = strings.Join(templateLines, "\n")
			} else {
				templateLines = append(templateLines, strings.TrimPrefix(strings.TrimPrefix(line, "  "), "\t"))
				continue
			}
		}

		// Парсинг ключ: значение
		if strings.Contains(trimmed, ":") {
			parts := strings.SplitN(trimmed, ":", 2)
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])

			// Убираем кавычки
			value = strings.Trim(value, "\"'")

			switch key {
			case "name":
				if currentSection == "parameters" && currentParam != nil {
					currentParam.Name = value
				} else {
					skill.Name = value
					currentSection = ""
				}
			case "description":
				if currentSection == "parameters" && currentParam != nil {
					currentParam.Description = value
				} else {
					skill.Description = value
				}
			case "version":
				skill.Version = value
			case "author":
				skill.Author = value
			case "endpoint":
				skill.Endpoint = value
				currentSection = ""
			case "method":
				skill.Method = value
			case "type":
				if currentParam != nil {
					currentParam.Type = value
				}
			case "required":
				if currentParam != nil {
					currentParam.Required = value == "true"
				}
			case "default":
				if currentParam != nil {
					currentParam.Default = value
				}
			case "parameters":
				currentSection = "parameters"
			case "tags":
				currentSection = "tags"
			case "agents":
				currentSection = "agents"
			case "template":
				if value == "|" || value == "" {
					inTemplate = true
					templateLines = nil
				} else {
					skill.Template = value
				}
			}

			// Элемент списка параметров (начинается с "- name:")
			if strings.HasPrefix(trimmed, "- name:") && currentSection == "parameters" {
				if currentParam != nil {
					skill.Parameters = append(skill.Parameters, *currentParam)
				}
				currentParam = &SkillParameter{
					Name: strings.TrimSpace(strings.TrimPrefix(trimmed, "- name:")),
				}
				continue
			}
		}

		// Элементы списков (теги, агенты)
		if strings.HasPrefix(trimmed, "- ") {
			item := strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
			item = strings.Trim(item, "\"'")
			switch currentSection {
			case "tags":
				skill.Tags = append(skill.Tags, item)
			case "agents":
				skill.Agents = append(skill.Agents, item)
			case "parameters":
				if currentParam == nil {
					currentParam = &SkillParameter{}
				}
			}
		}
	}

	// Финализация последнего шаблона и параметра
	if inTemplate && len(templateLines) > 0 {
		skill.Template = strings.Join(templateLines, "\n")
	}
	if currentParam != nil && currentParam.Name != "" {
		skill.Parameters = append(skill.Parameters, *currentParam)
	}

	if skill.Name == "" {
		return nil, fmt.Errorf("навык без имени в файле %s", filePath)
	}

	return skill, nil
}

// GetSkill — возвращает навык по имени.
// Потокобезопасный метод для получения навыка из кэша.
func (l *SkillLoader) GetSkill(name string) (*Skill, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	skill, ok := l.skills[name]
	return skill, ok
}

// GetSkillsForAgent — возвращает все навыки, доступные указанному агенту.
// agentRole — роль агента: "admin", "coder", "novice".
func (l *SkillLoader) GetSkillsForAgent(agentRole string) []*Skill {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var result []*Skill
	role := strings.ToLower(agentRole)
	for _, skill := range l.skills {
		for _, agent := range skill.Agents {
			if strings.ToLower(agent) == role {
				result = append(result, skill)
				break
			}
		}
	}
	return result
}

// GetAllSkills — возвращает список всех загруженных навыков.
func (l *SkillLoader) GetAllSkills() []*Skill {
	l.mu.RLock()
	defer l.mu.RUnlock()

	result := make([]*Skill, 0, len(l.skills))
	for _, skill := range l.skills {
		result = append(result, skill)
	}
	return result
}

// SearchSkills — ищет навыки по тегам или ключевым словам в описании.
func (l *SkillLoader) SearchSkills(query string) []*Skill {
	l.mu.RLock()
	defer l.mu.RUnlock()

	q := strings.ToLower(query)
	var result []*Skill
	for _, skill := range l.skills {
		// Поиск по имени
		if strings.Contains(strings.ToLower(skill.Name), q) {
			result = append(result, skill)
			continue
		}
		// Поиск по описанию
		if strings.Contains(strings.ToLower(skill.Description), q) {
			result = append(result, skill)
			continue
		}
		// Поиск по тегам
		for _, tag := range skill.Tags {
			if strings.Contains(strings.ToLower(tag), q) {
				result = append(result, skill)
				break
			}
		}
	}
	return result
}

// ExecuteSkill — выполняет навык с указанными параметрами.
// Подставляет значения параметров в шаблон запроса,
// отправляет HTTP-запрос на эндпоинт и возвращает результат.
func (l *SkillLoader) ExecuteSkill(skillName string, params map[string]interface{}) (*SkillExecutionResult, error) {
	skill, ok := l.GetSkill(skillName)
	if !ok {
		return nil, fmt.Errorf("навык '%s' не найден", skillName)
	}

	// Подставляем параметры в шаблон
	body := skill.Template
	for _, param := range skill.Parameters {
		placeholder := "{{" + param.Name + "}}"
		if val, exists := params[param.Name]; exists {
			body = strings.ReplaceAll(body, placeholder, fmt.Sprintf("%v", val))
		} else if param.Default != nil {
			body = strings.ReplaceAll(body, placeholder, fmt.Sprintf("%v", param.Default))
		} else if param.Required {
			return nil, fmt.Errorf("обязательный параметр '%s' не указан для навыка '%s'", param.Name, skillName)
		} else {
			body = strings.ReplaceAll(body, placeholder, "")
		}
	}

	// Создаём HTTP-запрос
	method := strings.ToUpper(skill.Method)
	if method == "" {
		method = "POST"
	}

	req, err := http.NewRequest(method, skill.Endpoint, bytes.NewBufferString(body))
	if err != nil {
		return nil, fmt.Errorf("ошибка создания запроса для навыка '%s': %w", skillName, err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Выполняем запрос
	resp, err := l.httpClient.Do(req)
	if err != nil {
		return &SkillExecutionResult{
			Success: false,
			Error:   fmt.Sprintf("Ошибка выполнения навыка '%s': %v", skillName, err),
		}, nil
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return &SkillExecutionResult{
			Success:    false,
			StatusCode: resp.StatusCode,
			Error:      fmt.Sprintf("Ошибка чтения ответа: %v", err),
		}, nil
	}

	return &SkillExecutionResult{
		Success:    resp.StatusCode >= 200 && resp.StatusCode < 300,
		StatusCode: resp.StatusCode,
		Body:       string(respBody),
	}, nil
}

// ToJSON — сериализует навык в JSON для передачи агенту.
func (s *Skill) ToJSON() (string, error) {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// SkillCount — возвращает количество загруженных навыков.
func (l *SkillLoader) SkillCount() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.skills)
}
