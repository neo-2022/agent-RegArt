// Package persistence — реализация репозиториев через GORM (PostgreSQL).
//
// Содержит конкретные реализации интерфейсов из пакета domain,
// работающие с PostgreSQL через ORM-библиотеку GORM.
package persistence

import (
	"github.com/neo-2022/openclaw-memory/agent-service/internal/domain"
	"gorm.io/gorm"
)

// GormAgentRepository — реализация AgentRepository через GORM.
// Работает с таблицей agents в PostgreSQL.
type GormAgentRepository struct {
	db *gorm.DB // Подключение к БД через GORM
}

// Создаёт репозиторий агентов с указанным подключением к БД.
func NewGormAgentRepository(db *gorm.DB) *GormAgentRepository {
	return &GormAgentRepository{db: db}
}

// Найти агента по имени в БД.
// Возвращает указатель на Agent или ошибку, если агент не найден.
func (r *GormAgentRepository) GetByName(name string) (*domain.Agent, error) {
	var agent domain.Agent
	err := r.db.Where("name = ?", name).First(&agent).Error
	if err != nil {
		return nil, err
	}
	return &agent, nil
}

// Сохранить или обновить агента в БД.
// Если агент с таким ID существует — обновляет, иначе — создаёт новую запись.
func (r *GormAgentRepository) Save(agent *domain.Agent) error {
	return r.db.Save(agent).Error
}

// Получить список всех агентов из БД.
func (r *GormAgentRepository) List() ([]domain.Agent, error) {
	var agents []domain.Agent
	err := r.db.Find(&agents).Error
	return agents, err
}

// Создать агента по умолчанию (admin), если в БД нет ни одного.
// Используется при первом запуске системы для инициализации.
func (r *GormAgentRepository) CreateDefault() error {
	var count int64
	r.db.Model(&domain.Agent{}).Count(&count)
	if count > 0 {
		return nil
	}
	defaultAgent := &domain.Agent{
		Name:   "admin",
		Prompt: "You are a helpful assistant.",
	}
	return r.db.Create(defaultAgent).Error
}
