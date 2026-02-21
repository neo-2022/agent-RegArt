package tools

import (
	"strings"

	"github.com/neo-2022/openclaw-memory/agent-service/internal/llm"
)

// GetToolsForAgent — выбирает набор инструментов для агента в зависимости от роли И модели.
//
// КЛЮЧЕВОЙ ПРИНЦИП:
//   - Слабая модель (3B и меньше) получает ТОЛЬКО составные скилы (LEGO-блоки).
//     Она не может выстроить цепочку из 15+ базовых инструментов.
//   - Сильная модель (7B+) получает базовые + оркестрационные инструменты.
//     Она сама построит нужную цепочку.
//
// Параметр modelName используется для определения размера модели.
// Если modelName пустой — считаем модель слабой (безопасный дефолт).
func GetToolsForAgent(agentName string, modelName ...string) []llm.Tool {
	// Определяем, слабая ли модель (3B и меньше)
	isWeakModel := true
	if len(modelName) > 0 && modelName[0] != "" {
		m := modelName[0]
		// Модели 7B+ считаются сильными: llama3, mistral, qwen2.5:7b, qwen2.5:14b и т.д.
		// Модели 3B и меньше считаются слабыми: qwen2.5:3b, qwen2.5:1.5b, phi-3-mini и т.д.
		// Также все облачные модели (через OpenRouter) считаются сильными.
		isWeakModel = isSmallModel(m)
	}

	if agentName == "admin" {
		if isWeakModel {
			// Слабая модель: ТОЛЬКО составные скилы (LEGO-блоки).
			// Модель вызывает 3-5 скилов вместо 15+ базовых инструментов.
			return GetCompoundSkillTools()
		}
		// Сильная модель: базовые + оркестрация. Сама строит цепочку.
		base := GetBaseTools()
		base = append(base, GetOrchestratorTools()...)
		return base
	}

	// Для coder и novice — только базовые инструменты.
	return GetBaseTools()
}

// isSmallModel — определяет, является ли модель слабой (3B и меньше).
// Слабые модели не могут выстроить длинные цепочки tool calls.
// Все облачные модели считаются сильными (они обычно 70B+).
// Ollama модели с "3b", "1.5b", "0.5b" в имени — слабые.
func isSmallModel(modelName string) bool {
	// Облачные модели через OpenRouter всегда сильные
	if strings.Contains(modelName, "/") {
		return false
	}
	// Проверяем известные маленькие размеры в имени модели
	smallSuffixes := []string{":3b", ":1.5b", ":0.5b", ":1b", ":2b", "-3b", "-1.5b", "-0.5b"}
	lower := strings.ToLower(modelName)
	for _, suffix := range smallSuffixes {
		if strings.Contains(lower, suffix) {
			return true
		}
	}
	// Если размер не указан — считаем сильной (на всякий случай)
	return false
}

// GetCompoundSkillTools — составные скилы-подстраховки для слабых моделей.
// Каждый скил объединяет несколько базовых инструментов в один вызов.
// Умная модель (7B+) предпочтёт базовые инструменты и сама построит цепочку.
// Слабая модель (3B) вызовет составной скил одним действием.
// В описании каждого скила указано: "Подстраховка — используй только если не можешь сделать это пошагово через execute/write/read."
// GetCompoundSkillTools — универсальные LEGO-блоки для агента-администратора.
//
// ВАЖНЫЙ ПРИНЦИП: каждый скил — это подстраховка для слабых моделей.
// Умная модель (7B+) ДОЛЖНА предпочитать базовые инструменты (execute, write, read и т.д.)
// и самостоятельно строить цепочку вызовов. Слабая модель (3B) не может выстроить
// длинную цепочку, поэтому вызовет составной скил одним действием.
//
// В описании КАЖДОГО скила есть фраза:
// "ПРИОРИТЕТ: сначала попробуй решить задачу пошагово через базовые инструменты.
// Используй этот скил ТОЛЬКО если не можешь."
//
// Скилы универсальные — подходят для ЛЮБЫХ задач администратора:
// миграция, мониторинг, диагностика, деплой, настройка, аудит и т.д.
func GetCompoundSkillTools() []llm.Tool {
	return []llm.Tool{
		// =====================================================================
		// БЛОК 1: Системные — аудит, проверка версий, диагностика
		// =====================================================================
		{
			Type: "function",
			Function: llm.FunctionDefinition{
				Name:        "full_system_report",
				Description: "Универсальный LEGO-блок: собрать полный отчёт о системе за один вызов. Выполняет sysinfo + sysload + cputemp + df -h + free -m + uname -a + uptime. ПРИОРИТЕТ: сначала попробуй собрать информацию пошагово через sysinfo/sysload/execute. Используй этот скил ТОЛЬКО если не можешь.",
				Parameters: map[string]any{
					"type":       "object",
					"properties": map[string]any{},
				},
			},
		},
		{
			Type: "function",
			Function: llm.FunctionDefinition{
				Name:        "check_stack",
				Description: "Универсальный LEGO-блок: проверить установленные версии технологического стека. Проверяет наличие и версии указанных программ (go, node, npm, psql, python3, docker, git и др.). ПРИОРИТЕТ: сначала попробуй проверить пошагово через execute('go version'), execute('node --version') и т.д. Используй этот скил ТОЛЬКО если не можешь.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"programs": map[string]any{
							"type": "array",
							"items": map[string]any{
								"type": "string",
							},
							"description": "Список программ для проверки (например: ['go', 'node', 'psql', 'python3', 'docker', 'git', 'nginx'])",
						},
					},
					"required": []string{"programs"},
				},
			},
		},
		{
			Type: "function",
			Function: llm.FunctionDefinition{
				Name:        "diagnose_service",
				Description: "Универсальный LEGO-блок: диагностика сервиса. Проверяет: порт занят ли, процесс работает ли, HTTP-ответ, последние логи. ПРИОРИТЕТ: сначала попробуй диагностировать пошагово через execute('ss -tlnp'), execute('curl ...') и т.д. Используй этот скил ТОЛЬКО если не можешь.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"service_name": map[string]any{
							"type":        "string",
							"description": "Название сервиса для диагностики (например: agent-service, postgresql, nginx)",
						},
						"port": map[string]any{
							"type":        "number",
							"description": "Порт сервиса для проверки (например: 8083, 5432, 80)",
						},
						"health_url": map[string]any{
							"type":        "string",
							"description": "URL для проверки здоровья (например: http://localhost:8083/health). Если не указан — проверяется только порт.",
						},
					},
					"required": []string{"service_name", "port"},
				},
			},
		},
		// =====================================================================
		// БЛОК 2: Интернет — поиск, проверка доступности
		// =====================================================================
		{
			Type: "function",
			Function: llm.FunctionDefinition{
				Name:        "web_research",
				Description: "Универсальный LEGO-блок: поиск информации в интернете по теме. Ищет через internet_search + загружает текст лучших результатов через browser_get_text. Возвращает структурированную сводку. ПРИОРИТЕТ: сначала попробуй использовать internet_search и browser_get_text пошагово. Используй этот скил ТОЛЬКО если не можешь.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"topic": map[string]any{
							"type":        "string",
							"description": "Тема для исследования (например: 'системные требования Go 1.22 + PostgreSQL 16 + React 18')",
						},
						"max_sources": map[string]any{
							"type":        "number",
							"description": "Максимальное количество источников для загрузки (по умолчанию 3)",
						},
					},
					"required": []string{"topic"},
				},
			},
		},
		{
			Type: "function",
			Function: llm.FunctionDefinition{
				Name:        "check_resources_batch",
				Description: "Универсальный LEGO-блок: проверить доступность нескольких URL/ресурсов за один вызов. Проверяет DNS, TCP, HTTP для каждого URL. ПРИОРИТЕТ: сначала попробуй проверить пошагово через check_url_access для каждого URL. Используй этот скил ТОЛЬКО если не можешь.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"urls": map[string]any{
							"type": "array",
							"items": map[string]any{
								"type": "string",
							},
							"description": "Список URL для проверки доступности",
						},
					},
					"required": []string{"urls"},
				},
			},
		},
		// =====================================================================
		// БЛОК 3: Команда — статус агентов, делегирование
		// =====================================================================
		{
			Type: "function",
			Function: llm.FunctionDefinition{
				Name:        "team_status",
				Description: "Универсальный LEGO-блок: получить статус ВСЕХ агентов (admin, coder, novice) за один вызов. Показывает модель, провайдера, промпт и готовность каждого. ПРИОРИТЕТ: сначала попробуй вызвать get_agent_info для каждого агента отдельно. Используй этот скил ТОЛЬКО если не можешь.",
				Parameters: map[string]any{
					"type":       "object",
					"properties": map[string]any{},
				},
			},
		},
		{
			Type: "function",
			Function: llm.FunctionDefinition{
				Name:        "delegate_tasks",
				Description: "Универсальный LEGO-блок: поручить задачи нескольким агентам за один вызов. Отправляет задачу Кодеру И Послушнику одновременно, собирает оба ответа. ПРИОРИТЕТ: сначала попробуй вызвать call_coder и call_novice по отдельности. Используй этот скил ТОЛЬКО если не можешь.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"coder_task": map[string]any{
							"type":        "string",
							"description": "Задача для Кодера (программирование, скрипты, код). Оставь пустым если задача для Кодера не нужна.",
						},
						"novice_task": map[string]any{
							"type":        "string",
							"description": "Задача для Послушника (документация, инструкции, поиск). Оставь пустым если задача для Послушника не нужна.",
						},
					},
				},
			},
		},
		// =====================================================================
		// БЛОК 4: Файлы и отчёты
		// =====================================================================
		{
			Type: "function",
			Function: llm.FunctionDefinition{
				Name:        "generate_report",
				Description: "Универсальный LEGO-блок: записать текстовый отчёт в файл и проверить что он записался корректно. Выполняет write + read + проверку размера. ПРИОРИТЕТ: сначала попробуй использовать write и read пошагово. Используй этот скил ТОЛЬКО если не можешь.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path": map[string]any{
							"type":        "string",
							"description": "Путь к файлу отчёта (например: /tmp/migration_report.txt)",
						},
						"content": map[string]any{
							"type":        "string",
							"description": "Текст отчёта для записи",
						},
						"title": map[string]any{
							"type":        "string",
							"description": "Заголовок отчёта (добавляется в начало файла)",
						},
					},
					"required": []string{"path", "content"},
				},
			},
		},
		{
			Type: "function",
			Function: llm.FunctionDefinition{
				Name:        "create_script",
				Description: "Универсальный LEGO-блок: создать исполняемый bash-скрипт. Записывает файл и делает chmod +x. ПРИОРИТЕТ: сначала попробуй использовать write + execute('chmod +x') пошагово. Используй этот скил ТОЛЬКО если не можешь.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path": map[string]any{
							"type":        "string",
							"description": "Путь для скрипта (например, /tmp/deploy.sh)",
						},
						"content": map[string]any{
							"type":        "string",
							"description": "Содержимое bash-скрипта (начинай с #!/bin/bash)",
						},
					},
					"required": []string{"path", "content"},
				},
			},
		},
		// =====================================================================
		// БЛОК 5: Утилиты — команды, cron, проекты
		// =====================================================================
		{
			Type: "function",
			Function: llm.FunctionDefinition{
				Name:        "run_commands",
				Description: "Универсальный LEGO-блок: выполнить несколько bash-команд последовательно за один вызов. ПРИОРИТЕТ: сначала попробуй вызвать execute для каждой команды отдельно. Используй этот скил ТОЛЬКО если не можешь.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"commands": map[string]any{
							"type": "array",
							"items": map[string]any{
								"type": "string",
							},
							"description": "Список bash-команд для последовательного выполнения",
						},
					},
					"required": []string{"commands"},
				},
			},
		},
		{
			Type: "function",
			Function: llm.FunctionDefinition{
				Name:        "setup_cron_job",
				Description: "Универсальный LEGO-блок: добавить задачу в crontab. ПРИОРИТЕТ: сначала попробуй добавить через execute('crontab ...') вручную. Используй этот скил ТОЛЬКО если не можешь.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"schedule": map[string]any{
							"type":        "string",
							"description": "Расписание в формате cron (например, '*/30 * * * *')",
						},
						"command": map[string]any{
							"type":        "string",
							"description": "Команда для выполнения по расписанию",
						},
					},
					"required": []string{"schedule", "command"},
				},
			},
		},
		{
			Type: "function",
			Function: llm.FunctionDefinition{
				Name:        "setup_git_automation",
				Description: "Универсальный LEGO-блок: настроить полную git-автоматизацию для проекта (init, autocommit, backup, crontab). ПРИОРИТЕТ: сначала попробуй настроить пошагово через execute/write. Используй этот скил ТОЛЬКО если не можешь.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"project_path": map[string]any{
							"type":        "string",
							"description": "Путь к директории проекта",
						},
						"backup_path": map[string]any{
							"type":        "string",
							"description": "Путь для бэкапов",
						},
						"autocommit_minutes": map[string]any{
							"type":        "number",
							"description": "Интервал автокоммитов в минутах (по умолчанию 30)",
						},
						"backup_schedule": map[string]any{
							"type":        "string",
							"description": "Расписание бэкапа в формате cron (по умолчанию '0 0 * * *')",
						},
					},
					"required": []string{"project_path", "backup_path"},
				},
			},
		},
		{
			Type: "function",
			Function: llm.FunctionDefinition{
				Name:        "project_init",
				Description: "Универсальный LEGO-блок: инициализировать новый проект (mkdir, README, .gitignore, git init). ПРИОРИТЕТ: сначала попробуй сделать пошагово через execute/write. Используй этот скил ТОЛЬКО если не можешь.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path": map[string]any{
							"type":        "string",
							"description": "Путь для нового проекта",
						},
						"name": map[string]any{
							"type":        "string",
							"description": "Название проекта",
						},
						"description": map[string]any{
							"type":        "string",
							"description": "Описание проекта",
						},
					},
					"required": []string{"path", "name"},
				},
			},
		},
		// =====================================================================
		// БЛОК 6: Установка ПО
		// =====================================================================
		{
			Type: "function",
			Function: llm.FunctionDefinition{
				Name:        "install_packages",
				Description: "Универсальный LEGO-блок: установить пакеты через apt/npm/pip. Автоматически определяет менеджер по названию пакетов или принимает явное указание. ПРИОРИТЕТ: сначала попробуй установить пошагово через execute('apt install ...'). Используй этот скил ТОЛЬКО если не можешь.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"packages": map[string]any{
							"type": "array",
							"items": map[string]any{
								"type": "string",
							},
							"description": "Список пакетов для установки",
						},
						"manager": map[string]any{
							"type":        "string",
							"description": "Менеджер пакетов: apt, npm, pip (по умолчанию apt)",
						},
					},
					"required": []string{"packages"},
				},
			},
		},
	}
}

func GetOrchestratorTools() []llm.Tool {
	return []llm.Tool{
		{
			Type: "function",
			Function: llm.FunctionDefinition{
				Name:        "call_coder",
				Description: "Вызвать агента Coder для выполнения задачи, связанной с программированием.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"task": map[string]any{
							"type":        "string",
							"description": "Задача для кодера",
						},
					},
					"required": []string{"task"},
				},
			},
		},
		{
			Type: "function",
			Function: llm.FunctionDefinition{
				Name:        "call_novice",
				Description: "Вызвать агента Novice для простых вопросов или поиска информации.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"task": map[string]any{
							"type":        "string",
							"description": "Задача для послушника",
						},
					},
					"required": []string{"task"},
				},
			},
		},
		{
			Type: "function",
			Function: llm.FunctionDefinition{
				Name:        "debug_code",
				Description: "Запустить код/скрипт и вернуть stdout/stderr для отладки. Админ может дебажить код на ПК.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"file_path": map[string]any{
							"type":        "string",
							"description": "Путь к файлу для запуска",
						},
						"args": map[string]any{
							"type":        "string",
							"description": "Аргументы командной строки (опционально)",
						},
					},
					"required": []string{"file_path"},
				},
			},
		},
		{
			Type: "function",
			Function: llm.FunctionDefinition{
				Name:        "edit_file",
				Description: "Редактировать файл на ПК: заменить старый текст на новый в указанном файле.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"file_path": map[string]any{
							"type":        "string",
							"description": "Путь к файлу",
						},
						"old_text": map[string]any{
							"type":        "string",
							"description": "Текст для замены",
						},
						"new_text": map[string]any{
							"type":        "string",
							"description": "Новый текст",
						},
					},
					"required": []string{"file_path", "old_text", "new_text"},
				},
			},
		},
		{
			Type: "function",
			Function: llm.FunctionDefinition{
				Name:        "delete",
				Description: "Удалить файл на ПК.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path": map[string]any{
							"type":        "string",
							"description": "Путь к файлу для удаления",
						},
					},
					"required": []string{"path"},
				},
			},
		},
		{
			Type: "function",
			Function: llm.FunctionDefinition{
				Name:        "configure_agent",
				Description: "Настроить агента: изменить модель, провайдера или промпт. Админ может настраивать Кодера и Послушника, подбирая правильные модели на их роли.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"agent_name": map[string]any{
							"type":        "string",
							"description": "Имя агента для настройки (coder, novice, admin)",
						},
						"model": map[string]any{
							"type":        "string",
							"description": "Новая модель для агента (опционально)",
						},
						"provider": map[string]any{
							"type":        "string",
							"description": "Провайдер модели: ollama, openai, anthropic, yandexgpt, gigachat (опционально)",
						},
						"prompt": map[string]any{
							"type":        "string",
							"description": "Новый системный промпт для агента (опционально)",
						},
					},
					"required": []string{"agent_name"},
				},
			},
		},
		{
			Type: "function",
			Function: llm.FunctionDefinition{
				Name:        "get_agent_info",
				Description: "Получить полную информацию об агенте: текущая модель, провайдер, промпт, поддержка инструментов.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"agent_name": map[string]any{
							"type":        "string",
							"description": "Имя агента (admin, coder, novice)",
						},
					},
					"required": []string{"agent_name"},
				},
			},
		},
		{
			Type: "function",
			Function: llm.FunctionDefinition{
				Name:        "list_models_for_role",
				Description: "Получить список доступных моделей с рекомендациями для указанной роли агента. Показывает какие модели подходят, а какие нет.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"role": map[string]any{
							"type":        "string",
							"description": "Роль агента: admin, coder, novice",
						},
					},
					"required": []string{"role"},
				},
			},
		},
		{
			Type: "function",
			Function: llm.FunctionDefinition{
				Name:        "view_logs",
				Description: "Просмотреть системные логи ошибок и событий из всех микросервисов. Можно фильтровать по уровню (error, warn, info) и сервису (agent-service, tools-service, memory-service, api-gateway).",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"level": map[string]any{
							"type":        "string",
							"description": "Фильтр по уровню: error, warn, info, debug (опционально)",
						},
						"service": map[string]any{
							"type":        "string",
							"description": "Фильтр по сервису: agent-service, tools-service, memory-service, api-gateway (опционально)",
						},
						"limit": map[string]any{
							"type":        "number",
							"description": "Количество записей (по умолчанию 20)",
						},
					},
				},
			},
		},
		// ============================================================================
		// Инструменты browser-service (MCP-микросервис на порту 8084)
		// ============================================================================

		// --- Навигация и контент ---
		{
			Type: "function",
			Function: llm.FunctionDefinition{
				Name:        "browser_get_dom",
				Description: "Получить полный DOM-контент веб-страницы через headless Chrome. Загружает страницу, выполняет весь JavaScript (SPA, React, Vue) и возвращает итоговый HTML после рендеринга. Используется для парсинга, анализа контента, извлечения данных.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"url": map[string]any{
							"type":        "string",
							"description": "URL страницы для получения DOM (например, https://ya.ru)",
						},
					},
					"required": []string{"url"},
				},
			},
		},
		{
			Type: "function",
			Function: llm.FunctionDefinition{
				Name:        "browser_open_visible",
				Description: "Открыть URL в видимом браузере пользователя. Используй когда пользователь говорит «покажи мне» — страница откроется в GUI-браузере. Не для фоновой работы — только для демонстрации пользователю.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"url": map[string]any{
							"type":        "string",
							"description": "URL для открытия в видимом браузере",
						},
					},
					"required": []string{"url"},
				},
			},
		},
		{
			Type: "function",
			Function: llm.FunctionDefinition{
				Name:        "browser_screenshot",
				Description: "Сделать скриншот веб-страницы через headless Chrome. Сохраняет PNG-файл. Можно указать размер окна и путь сохранения. Полезно для визуальной проверки страниц, создания превью, мониторинга.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"url": map[string]any{
							"type":        "string",
							"description": "URL страницы для скриншота",
						},
						"output_path": map[string]any{
							"type":        "string",
							"description": "Путь для сохранения PNG (если не указан — временный файл)",
						},
						"window_size": map[string]any{
							"type":        "string",
							"description": "Размер окна «ширина,высота» (по умолчанию 1920,1080)",
						},
					},
					"required": []string{"url"},
				},
			},
		},
		{
			Type: "function",
			Function: llm.FunctionDefinition{
				Name:        "browser_pdf",
				Description: "Сохранить веб-страницу как PDF через headless Chrome. Полный рендеринг страницы с JavaScript. Полезно для сохранения документов, отчётов, статей.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"url": map[string]any{
							"type":        "string",
							"description": "URL страницы для конвертации в PDF",
						},
						"output_path": map[string]any{
							"type":        "string",
							"description": "Путь для сохранения PDF (если не указан — временный файл)",
						},
					},
					"required": []string{"url"},
				},
			},
		},
		{
			Type: "function",
			Function: llm.FunctionDefinition{
				Name:        "browser_get_text",
				Description: "Получить текстовое содержимое веб-страницы без HTML-тегов. Полезно для быстрого чтения контента, анализа текста, суммаризации.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"url": map[string]any{
							"type":        "string",
							"description": "URL страницы",
						},
					},
					"required": []string{"url"},
				},
			},
		},
		{
			Type: "function",
			Function: llm.FunctionDefinition{
				Name:        "browser_get_title",
				Description: "Получить заголовок (title) веб-страницы.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"url": map[string]any{
							"type":        "string",
							"description": "URL страницы",
						},
					},
					"required": []string{"url"},
				},
			},
		},
		{
			Type: "function",
			Function: llm.FunctionDefinition{
				Name:        "browser_execute_js",
				Description: "Выполнить произвольный JavaScript-код на веб-странице через headless Chrome. Можно извлекать данные, кликать элементы, заполнять формы. Аналог CDP Runtime.evaluate().",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"url": map[string]any{
							"type":        "string",
							"description": "URL страницы (контекст выполнения)",
						},
						"js_code": map[string]any{
							"type":        "string",
							"description": "JavaScript-код для выполнения",
						},
					},
					"required": []string{"url", "js_code"},
				},
			},
		},
		{
			Type: "function",
			Function: llm.FunctionDefinition{
				Name:        "browser_detect_captcha",
				Description: "Проверить веб-страницу на наличие CAPTCHA (reCAPTCHA, hCaptcha, Yandex SmartCaptcha, Cloudflare Turnstile). Если CAPTCHA обнаружена — сообщи пользователю, ты не можешь её решить.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"url": map[string]any{
							"type":        "string",
							"description": "URL страницы для проверки",
						},
					},
					"required": []string{"url"},
				},
			},
		},

		// --- Ввод и управление (xdotool/wmctrl) ---
		{
			Type: "function",
			Function: llm.FunctionDefinition{
				Name:        "input_key_press",
				Description: "Нажать клавишу или комбинацию клавиш через xdotool. Поддерживает все клавиши: Return, Tab, Escape, F1-F12, ctrl+c, alt+F4, super и т.д.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"keys": map[string]any{
							"type":        "string",
							"description": "Клавиша или комбинация (например: Return, ctrl+c, alt+F4, super+l)",
						},
					},
					"required": []string{"keys"},
				},
			},
		},
		{
			Type: "function",
			Function: llm.FunctionDefinition{
				Name:        "input_type_text",
				Description: "Ввести текст посимвольно через xdotool. Имитирует реальный набор на клавиатуре. Можно указать задержку между символами.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"text": map[string]any{
							"type":        "string",
							"description": "Текст для ввода",
						},
						"delay": map[string]any{
							"type":        "number",
							"description": "Задержка между символами в миллисекундах (по умолчанию 50)",
						},
					},
					"required": []string{"text"},
				},
			},
		},
		{
			Type: "function",
			Function: llm.FunctionDefinition{
				Name:        "input_mouse_click",
				Description: "Клик мышью по координатам через xdotool. Левая, средняя или правая кнопка. Можно указать количество кликов (двойной клик и т.д.).",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"x": map[string]any{
							"type":        "number",
							"description": "Координата X",
						},
						"y": map[string]any{
							"type":        "number",
							"description": "Координата Y",
						},
						"button": map[string]any{
							"type":        "number",
							"description": "Кнопка мыши: 1=левая, 2=средняя, 3=правая (по умолчанию 1)",
						},
					},
					"required": []string{"x", "y"},
				},
			},
		},
		{
			Type: "function",
			Function: llm.FunctionDefinition{
				Name:        "input_mouse_scroll",
				Description: "Прокрутка колеса мыши через xdotool. Вверх или вниз, указанное количество шагов.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"direction": map[string]any{
							"type":        "string",
							"description": "Направление: up (вверх) или down (вниз)",
						},
						"clicks": map[string]any{
							"type":        "number",
							"description": "Количество шагов прокрутки (по умолчанию 3)",
						},
					},
					"required": []string{"direction"},
				},
			},
		},
		{
			Type: "function",
			Function: llm.FunctionDefinition{
				Name:        "input_tab_action",
				Description: "Управление вкладками браузера через горячие клавиши: new (новая), close (закрыть), next (следующая), prev (предыдущая), reopen (восстановить), reload (обновить), goto (перейти к N-й).",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"action": map[string]any{
							"type":        "string",
							"description": "Действие: new, close, next, prev, reopen, reload, goto",
						},
						"param": map[string]any{
							"type":        "string",
							"description": "Дополнительный параметр (номер вкладки для goto)",
						},
					},
					"required": []string{"action"},
				},
			},
		},
		{
			Type: "function",
			Function: llm.FunctionDefinition{
				Name:        "input_window_action",
				Description: "Управление окнами через wmctrl/xdotool: list (список), activate (фокус), close (закрыть), minimize, maximize, fullscreen, move, resize.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"action": map[string]any{
							"type":        "string",
							"description": "Действие: list, activate, close, minimize, maximize, fullscreen, move, resize",
						},
						"target": map[string]any{
							"type":        "string",
							"description": "ID или заголовок окна",
						},
						"params": map[string]any{
							"type":        "string",
							"description": "Параметры (x,y,w,h для move/resize)",
						},
					},
					"required": []string{"action"},
				},
			},
		},
		{
			Type: "function",
			Function: llm.FunctionDefinition{
				Name:        "input_clipboard",
				Description: "Операции с буфером обмена через xclip: copy (скопировать текст), paste (вставить), get (получить содержимое), clear (очистить).",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"action": map[string]any{
							"type":        "string",
							"description": "Действие: copy, paste, get, clear",
						},
						"text": map[string]any{
							"type":        "string",
							"description": "Текст для копирования (только для copy)",
						},
					},
					"required": []string{"action"},
				},
			},
		},

		// --- Поиск в интернете ---
		{
			Type: "function",
			Function: llm.FunctionDefinition{
				Name:        "internet_search",
				Description: "Поиск в интернете через бесплатные открытые инструменты (DuckDuckGo, SearXNG). Не требует API-ключей. Работает в РФ. Поддерживает русский язык. Автоматически выбирает лучший поисковик.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"query": map[string]any{
							"type":        "string",
							"description": "Поисковый запрос на любом языке",
						},
						"max_results": map[string]any{
							"type":        "number",
							"description": "Максимальное количество результатов (по умолчанию 10)",
						},
						"engine": map[string]any{
							"type":        "string",
							"description": "Предпочитаемый поисковик: duckduckgo, searxng, auto (по умолчанию auto)",
						},
					},
					"required": []string{"query"},
				},
			},
		},

		// --- Краулер (маскировка под роботов) ---
		{
			Type: "function",
			Function: llm.FunctionDefinition{
				Name:        "crawler_fetch",
				Description: "Загрузить контент веб-страницы с маскировкой под поискового робота (Googlebot, YandexBot, Bingbot). Помогает обойти блокировки. Режим auto автоматически пробует разные User-Agent.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"url": map[string]any{
							"type":        "string",
							"description": "URL для загрузки",
						},
						"mode": map[string]any{
							"type":        "string",
							"description": "Режим маскировки: googlebot, yandexbot, bingbot, mailru, normal, auto (по умолчанию auto)",
						},
					},
					"required": []string{"url"},
				},
			},
		},
		{
			Type: "function",
			Function: llm.FunctionDefinition{
				Name:        "crawler_robots_txt",
				Description: "Получить и проанализировать robots.txt сайта. Показывает какие разделы разрешены/запрещены для роботов.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"url": map[string]any{
							"type":        "string",
							"description": "Базовый URL сайта (например, https://example.com)",
						},
						"mode": map[string]any{
							"type":        "string",
							"description": "Режим робота: googlebot, yandexbot, bingbot (по умолчанию googlebot)",
						},
					},
					"required": []string{"url"},
				},
			},
		},

		// --- Проверка доступности URL ---
		{
			Type: "function",
			Function: llm.FunctionDefinition{
				Name:        "check_url_access",
				Description: "Проверить доступность URL с учётом санкций и блокировок. Выполняет цепочку проверок: DNS → TCP → TLS → HTTP. Обнаруживает геоблокировки, блокировки провайдером, CAPTCHA. Даёт рекомендации на русском.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"url": map[string]any{
							"type":        "string",
							"description": "URL для проверки доступности",
						},
					},
					"required": []string{"url"},
				},
			},
		},
		{
			Type: "function",
			Function: llm.FunctionDefinition{
				Name:        "check_multiple_urls",
				Description: "Проверить доступность нескольких URL одновременно. Каждый URL проходит полную цепочку проверок (DNS, TCP, TLS, HTTP).",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"urls": map[string]any{
							"type": "array",
							"items": map[string]any{
								"type": "string",
							},
							"description": "Список URL для проверки",
						},
					},
					"required": []string{"urls"},
				},
			},
		},
	}
}

func GetAllTools() []llm.Tool {
	return append(GetBaseTools(), GetOrchestratorTools()...)
}

func GetBaseTools() []llm.Tool {
	return []llm.Tool{
		{
			Type: "function",
			Function: llm.FunctionDefinition{
				Name:        "execute",
				Description: "Выполнить bash-команду. Разрешены только команды из белого списка.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"command": map[string]any{
							"type":        "string",
							"description": "Команда для выполнения",
						},
					},
					"required": []string{"command"},
				},
			},
		},
		{
			Type: "function",
			Function: llm.FunctionDefinition{
				Name:        "read",
				Description: "Прочитать содержимое файла.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path": map[string]any{
							"type":        "string",
							"description": "Путь к файлу",
						},
					},
					"required": []string{"path"},
				},
			},
		},
		{
			Type: "function",
			Function: llm.FunctionDefinition{
				Name:        "write",
				Description: "Записать содержимое в файл.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path": map[string]any{
							"type":        "string",
							"description": "Путь к файлу",
						},
						"content": map[string]any{
							"type":        "string",
							"description": "Содержимое для записи",
						},
					},
					"required": []string{"path", "content"},
				},
			},
		},
		{
			Type: "function",
			Function: llm.FunctionDefinition{
				Name:        "list",
				Description: "Показать содержимое директории. Если path пустой или '~' — показывает домашнюю директорию текущего пользователя. Поддерживает '~/subdir' для поддиректорий домашней папки.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path": map[string]any{
							"type":        "string",
							"description": "Путь к директории. Используй '~' для домашней директории, '~/Documents' для поддиректорий. Если не указан — домашняя директория.",
						},
					},
				},
			},
		},
		{
			Type: "function",
			Function: llm.FunctionDefinition{
				Name:        "sysinfo",
				Description: "Получить базовую информацию о системе: ОС, архитектура, имя хоста, домашняя директория (home_dir), имя пользователя (user).",
				Parameters: map[string]any{
					"type":       "object",
					"properties": map[string]any{},
				},
			},
		},
		{
			Type: "function",
			Function: llm.FunctionDefinition{
				Name:        "cputemp",
				Description: "Получить температуру процессора в градусах Цельсия.",
				Parameters: map[string]any{
					"type":       "object",
					"properties": map[string]any{},
				},
			},
		},
		{
			Type: "function",
			Function: llm.FunctionDefinition{
				Name:        "sysload",
				Description: "Получить информацию о загрузке системы (load average, память, диски).",
				Parameters: map[string]any{
					"type":       "object",
					"properties": map[string]any{},
				},
			},
		},
		{
			Type: "function",
			Function: llm.FunctionDefinition{
				Name:        "findapp",
				Description: "Найти .desktop файлы для приложения по имени.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"name": map[string]any{
							"type":        "string",
							"description": "Имя приложения",
						},
					},
					"required": []string{"name"},
				},
			},
		},
		{
			Type: "function",
			Function: llm.FunctionDefinition{
				Name:        "launchapp",
				Description: "Запустить приложение по .desktop файлу.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"desktop_file": map[string]any{
							"type":        "string",
							"description": "Путь к .desktop файлу",
						},
					},
					"required": []string{"desktop_file"},
				},
			},
		},
		{
			Type: "function",
			Function: llm.FunctionDefinition{
				Name:        "addautostart",
				Description: "Добавить приложение в автозагрузку по имени.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"app_name": map[string]any{
							"type":        "string",
							"description": "Имя приложения",
						},
					},
					"required": []string{"app_name"},
				},
			},
		},
	}
}
