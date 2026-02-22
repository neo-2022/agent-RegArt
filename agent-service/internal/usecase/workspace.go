package usecase

import "github.com/neo-2022/openclaw-memory/agent-service/internal/domain"

// WorkspaceUseCase — сценарий работы с рабочими пространствами.
// Оркестрирует операции создания, чтения, обновления и удаления рабочих пространств и связанных чатов.
type WorkspaceUseCase struct {
	workspaces domain.WorkspaceRepository // Репозиторий рабочих пространств
	chats      domain.ChatRepository      // Репозиторий чатов
}

// Создаёт новый экземпляр WorkspaceUseCase с заданными зависимостями.
func NewWorkspaceUseCase(
	workspaces domain.WorkspaceRepository,
	chats domain.ChatRepository,
) *WorkspaceUseCase {
	return &WorkspaceUseCase{
		workspaces: workspaces,
		chats:      chats,
	}
}

// Получить список всех рабочих пространств.
func (uc *WorkspaceUseCase) List() ([]domain.Workspace, error) {
	return uc.workspaces.List()
}

// Создать новое рабочее пространство с указанным именем и путём.
func (uc *WorkspaceUseCase) Create(name, path string) (*domain.Workspace, error) {
	ws := &domain.Workspace{Name: name, Path: path}
	if err := uc.workspaces.Create(ws); err != nil {
		return nil, err
	}
	return ws, nil
}

// Удалить рабочее пространство по ID.
func (uc *WorkspaceUseCase) Delete(id uint) error {
	return uc.workspaces.Delete(id)
}

// Получить список чатов в рабочем пространстве.
// Если workspaceID равен nil, возвращаются все чаты.
func (uc *WorkspaceUseCase) ListChats(workspaceID *uint) ([]domain.Chat, error) {
	return uc.chats.List(workspaceID)
}
