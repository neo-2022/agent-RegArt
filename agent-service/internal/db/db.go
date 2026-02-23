// Пакет db — инициализация подключения к PostgreSQL и автоматические миграции.
// Используется библиотека GORM (Go ORM) для работы с базой данных.
//
// Подключение настраивается через переменные окружения:
//   - DATABASE_URL — полная строка подключения (приоритетная)
//   - DB_HOST, DB_PORT, DB_USER, DB_PASSWORD, DB_NAME — отдельные параметры
//
// При запуске автоматически создаются/обновляются таблицы для всех моделей.
// Порядок миграций важен из-за внешних ключей: Chat → Agent → Message → остальные.
package db

import (
	"log"
	"os"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/neo-2022/openclaw-memory/agent-service/internal/models"
)

// getEnv — вспомогательная функция для чтения переменной окружения.
// Если переменная не задана или пуста, возвращает значение по умолчанию.
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// DB — глобальный экземпляр подключения к PostgreSQL через GORM.
// Инициализируется при вызове InitDB() и используется всеми хэндлерами и репозиториями.
var DB *gorm.DB

// InitDB — инициализирует подключение к PostgreSQL и выполняет автоматические миграции.
//
// Порядок действий:
//  1. Формирование DSN (Data Source Name) из переменных окружения.
//     Приоритет: DATABASE_URL > отдельные DB_* переменные > значения по умолчанию.
//  2. Подключение к PostgreSQL через GORM с логированием SQL-запросов (уровень Info).
//  3. Включение расширения uuid-ossp для генерации UUID (gen_random_uuid).
//  4. Автоматические миграции всех моделей в правильном порядке:
//     Chat → Agent → Message → PromptFile → ModelToolSupport → ProviderConfig → Workspace.
//     Порядок важен, так как Message зависит от Chat и Agent через внешние ключи.
//
// При ошибке подключения или миграции — программа завершается (log.Fatal).
func InitDB() {
	// Формируем строку подключения (DSN) из переменных окружения
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		// Если DATABASE_URL не задан — собираем DSN из отдельных параметров
		host := getEnv("DB_HOST", "localhost")
		port := getEnv("DB_PORT", "5432")
		user := getEnv("DB_USER", "agent_user")
		password := getEnv("DB_PASSWORD", "agent_password")
		dbname := getEnv("DB_NAME", "agent_db")
		dsn = "host=" + host + " user=" + user + " password=" + password + " dbname=" + dbname + " port=" + port + " sslmode=disable TimeZone=Europe/Moscow"
	}

	var err error
	// Открываем подключение к PostgreSQL с логированием SQL-запросов
	DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		log.Fatal("Ошибка подключения к базе данных:", err)
	}

	// Настройка пула соединений для production-нагрузки
	sqlDB, err := DB.DB()
	if err != nil {
		log.Fatal("Ошибка получения sql.DB:", err)
	}
	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(5 * time.Minute)
	sqlDB.SetConnMaxIdleTime(1 * time.Minute)
	log.Println("Пул соединений настроен: MaxOpen=25, MaxIdle=5, MaxLifetime=5m")

	// Включаем расширение uuid-ossp для генерации UUID в PostgreSQL
	DB.Exec("CREATE EXTENSION IF NOT EXISTS \"uuid-ossp\";")

	// Проверяем соединение
	if err := sqlDB.Ping(); err != nil {
		log.Fatal("Ошибка проверки соединения с БД:", err)
	}

	// Автоматические миграции в правильном порядке (учёт внешних ключей):
	// 1. Chat — базовая сущность, на которую ссылаются Message и Workspace
	if err := DB.AutoMigrate(&models.Chat{}); err != nil {
		log.Fatal("Ошибка миграции Chat:", err)
	}
	// 2. Agent — базовая сущность, на которую ссылаются Message
	if err := DB.AutoMigrate(&models.Agent{}); err != nil {
		log.Fatal("Ошибка миграции Agent:", err)
	}
	// 3. Message — зависит от Chat и Agent
	if err := DB.AutoMigrate(&models.Message{}); err != nil {
		log.Fatal("Ошибка миграции Message:", err)
	}
	// 4. PromptFile — независимая таблица для хранения файлов промптов
	if err := DB.AutoMigrate(&models.PromptFile{}); err != nil {
		log.Fatal("Ошибка миграции PromptFile:", err)
	}
	// 5. ModelToolSupport — кэш поддержки инструментов для моделей
	if err := DB.AutoMigrate(&models.ModelToolSupport{}); err != nil {
		log.Fatal("Ошибка миграции ModelToolSupport:", err)
	}
	// 6. ProviderConfig — настройки облачных LLM-провайдеров
	if err := DB.AutoMigrate(&models.ProviderConfig{}); err != nil {
		log.Fatal("Ошибка миграции ProviderConfig:", err)
	}
	// 7. Workspace — рабочие пространства (зависит от Chat и Agent)
	if err := DB.AutoMigrate(&models.Workspace{}); err != nil {
		log.Fatal("Ошибка миграции Workspace:", err)
	}
	// 8. SystemLog — централизованные логи ошибок и событий всех микросервисов
	if err := DB.AutoMigrate(&models.SystemLog{}); err != nil {
		log.Fatal("Ошибка миграции SystemLog:", err)
	}
	// 9. RagDocument — документы базы знаний RAG
	if err := DB.AutoMigrate(&models.RagDocument{}); err != nil {
		log.Fatal("Ошибка миграции RagDocument:", err)
	}

	log.Println("База данных подключена, миграции выполнены")
}
