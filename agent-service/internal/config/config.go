// Package config — централизованная конфигурация agent-service.
//
// Все параметры загружаются из переменных окружения с указанными значениями по умолчанию.
// Используется для единообразного доступа к настройкам из любого пакета сервиса.
package config

import "os"

// Config — структура конфигурации agent-service.
// Содержит все параметры подключения к БД, внешним сервисам и пути к ресурсам.
type Config struct {
	Port              string // Порт HTTP-сервера агента (по умолчанию 8083)
	DBHost            string // Хост PostgreSQL (по умолчанию localhost)
	DBPort            string // Порт PostgreSQL (по умолчанию 5432)
	DBUser            string // Пользователь PostgreSQL
	DBPassword        string // Пароль PostgreSQL
	DBName            string // Имя базы данных
	MemoryServiceURL  string // URL сервиса памяти (RAG)
	ToolsServiceURL   string // URL сервиса инструментов
	BrowserServiceURL string // URL сервиса браузера
	OllamaURL         string // URL Ollama API для LLM
	UploadsDir        string // Директория для загруженных файлов
	SkillsDir         string // Директория с пользовательскими скиллами
}

// Load — загружает конфигурацию из переменных окружения.
// Если переменная не задана, используется значение по умолчанию.
func Load() *Config {
	return &Config{
		Port:              getEnv("AGENT_PORT", "8083"),
		DBHost:            getEnv("DB_HOST", "localhost"),
		DBPort:            getEnv("DB_PORT", "5432"),
		DBUser:            getEnv("DB_USER", "agentcore"),
		DBPassword:        getEnv("DB_PASSWORD", "agentcore"),
		DBName:            getEnv("DB_NAME", "agentcore"),
		MemoryServiceURL:  getEnv("MEMORY_SERVICE_URL", "http://localhost:8001"),
		ToolsServiceURL:   getEnv("TOOLS_SERVICE_URL", "http://localhost:8082"),
		BrowserServiceURL: getEnv("BROWSER_SERVICE_URL", "http://localhost:8084"),
		OllamaURL:         getEnv("OLLAMA_URL", "http://127.0.0.1:11434"),
		UploadsDir:        getEnv("UPLOADS_DIR", "./uploads"),
		SkillsDir:         getEnv("SKILLS_DIR", "./skills"),
	}
}

// getEnv — возвращает значение переменной окружения или fallback, если не задана.
func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
