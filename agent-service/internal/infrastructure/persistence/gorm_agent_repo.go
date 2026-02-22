package persistence

import (
	"github.com/neo-2022/openclaw-memory/agent-service/internal/domain"
	"gorm.io/gorm"
)

type GormAgentRepository struct {
	db *gorm.DB
}

func NewGormAgentRepository(db *gorm.DB) *GormAgentRepository {
	return &GormAgentRepository{db: db}
}

func (r *GormAgentRepository) GetByName(name string) (*domain.Agent, error) {
	var agent domain.Agent
	err := r.db.Where("name = ?", name).First(&agent).Error
	if err != nil {
		return nil, err
	}
	return &agent, nil
}

func (r *GormAgentRepository) Save(agent *domain.Agent) error {
	return r.db.Save(agent).Error
}

func (r *GormAgentRepository) List() ([]domain.Agent, error) {
	var agents []domain.Agent
	err := r.db.Find(&agents).Error
	return agents, err
}

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
