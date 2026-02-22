package usecase

import "github.com/neo-2022/openclaw-memory/agent-service/internal/domain"

type WorkspaceUseCase struct {
	workspaces domain.WorkspaceRepository
	chats      domain.ChatRepository
}

func NewWorkspaceUseCase(
	workspaces domain.WorkspaceRepository,
	chats domain.ChatRepository,
) *WorkspaceUseCase {
	return &WorkspaceUseCase{
		workspaces: workspaces,
		chats:      chats,
	}
}

func (uc *WorkspaceUseCase) List() ([]domain.Workspace, error) {
	return uc.workspaces.List()
}

func (uc *WorkspaceUseCase) Create(name, path string) (*domain.Workspace, error) {
	ws := &domain.Workspace{Name: name, Path: path}
	if err := uc.workspaces.Create(ws); err != nil {
		return nil, err
	}
	return ws, nil
}

func (uc *WorkspaceUseCase) Delete(id uint) error {
	return uc.workspaces.Delete(id)
}

func (uc *WorkspaceUseCase) ListChats(workspaceID *uint) ([]domain.Chat, error) {
	return uc.chats.List(workspaceID)
}
