package auth

import (
	"testing"
)

func TestRoleAllowedCommand_Admin(t *testing.T) {
	cmds := []string{"ls", "rm", "kill", "docker", "reboot", "systemctl"}
	for _, cmd := range cmds {
		if !RoleAllowedCommand(RoleAdmin, cmd) {
			t.Errorf("admin должен иметь доступ к %s", cmd)
		}
	}
}

func TestRoleAllowedCommand_Viewer(t *testing.T) {
	allowed := []string{"ls", "cat", "grep", "ps", "df", "echo", "whoami"}
	for _, cmd := range allowed {
		if !RoleAllowedCommand(RoleViewer, cmd) {
			t.Errorf("viewer должен иметь доступ к %s", cmd)
		}
	}

	denied := []string{"mkdir", "cp", "mv", "docker", "apt", "kill", "systemctl"}
	for _, cmd := range denied {
		if RoleAllowedCommand(RoleViewer, cmd) {
			t.Errorf("viewer НЕ должен иметь доступ к %s", cmd)
		}
	}
}

func TestRoleAllowedCommand_Operator(t *testing.T) {
	allowed := []string{"ls", "cat", "mkdir", "cp", "docker", "git", "python"}
	for _, cmd := range allowed {
		if !RoleAllowedCommand(RoleOperator, cmd) {
			t.Errorf("operator должен иметь доступ к %s", cmd)
		}
	}

	denied := []string{"reboot", "shutdown", "systemctl", "kill", "mount"}
	for _, cmd := range denied {
		if RoleAllowedCommand(RoleOperator, cmd) {
			t.Errorf("operator НЕ должен иметь доступ к %s", cmd)
		}
	}
}

func TestRoleAllowedCommand_UnknownRole(t *testing.T) {
	if RoleAllowedCommand(Role("unknown"), "ls") {
		t.Error("неизвестная роль не должна иметь доступ ни к чему")
	}
}
