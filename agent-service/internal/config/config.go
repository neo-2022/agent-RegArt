package config

import "os"

type Config struct {
	Port              string
	DBHost            string
	DBPort            string
	DBUser            string
	DBPassword        string
	DBName            string
	MemoryServiceURL  string
	ToolsServiceURL   string
	BrowserServiceURL string
	OllamaURL         string
	UploadsDir        string
	SkillsDir         string
}

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
		OllamaURL:         getEnv("OLLAMA_URL", "http://localhost:11434"),
		UploadsDir:        getEnv("UPLOADS_DIR", "./uploads"),
		SkillsDir:         getEnv("SKILLS_DIR", "./skills"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
