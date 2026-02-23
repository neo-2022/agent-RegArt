// Package executor — безопасное выполнение команд в системе.
//
// Предоставляет функции для выполнения shell-команд с многоуровневой защитой:
//   - Белый список разрешённых команд (AllowedCommands)
//   - Чёрный список опасных команд (DangerousCommands)
//   - Блокировка деструктивных паттернов (BlockedPatterns)
//   - Поддержка составных команд через &&, ||, |, ;
//
// Используется tools-service для обработки запросов от агентов.
package executor

import (
	"fmt"
	"log/slog"
	"os/exec"
	"regexp"
	"strings"

	"github.com/neo-2022/openclaw-memory/tools-service/internal/execmode"
)

// extractSubCommands — извлекает имена всех команд из составной команды.
// Разбирает цепочки через &&, ||, |, ; и подстановки $(...)
// Возвращает список имён первых слов каждой подкоманды для проверки по белому списку.
func extractSubCommands(command string) []string {
	var cmds []string
	for _, sep := range []string{"&&", "||", "|", ";"} {
		command = strings.ReplaceAll(command, sep, "\n")
	}
	lines := strings.Split(command, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		line = strings.TrimLeft(line, "( ")
		parts := strings.Fields(line)
		if len(parts) > 0 {
			cmds = append(cmds, parts[0])
		}
	}
	return cmds
}

// AllowedCommands — белый список разрешённых команд (первые слова).
// Только команды из этого списка могут быть выполнены через API.
// Список включает: файловые операции, мониторинг, пакетные менеджеры,
// системные утилиты, средства разработки и сетевые инструменты.
var AllowedCommands = map[string]bool{
	"ls": true, "cat": true, "grep": true, "find": true,
	"ps": true, "top": true, "free": true, "df": true, "du": true,
	"echo": true, "mkdir": true, "rmdir": true, "touch": true,
	"cp": true, "mv": true,
	"python": true, "python3": true, "pip": true, "pip3": true,
	"node": true, "npm": true, "yarn": true, "git": true,
	"curl": true, "wget": true, "tar": true, "gzip": true,
	"gunzip": true, "zip": true, "unzip": true,
	"systemctl": true, "journalctl": true, "service": true,
	"kill": true, "pkill": true,
	"xdg-open": true, "sensors": true, "nvidia-smi": true,
	"apt": true, "apt-get": true, "dnf": true, "yum": true, "pacman": true,
	"lspci": true, "lshw": true, "dmidecode": true, "inxi": true,
	"lscpu": true, "lsusb": true, "hwinfo": true,
	"lsblk": true, "blkid": true, "fdisk": true,
	"parted": true, "smartctl": true,
	"reboot": true, "shutdown": true, "poweroff": true, "halt": true,
	"dpkg": true, "rpm": true, "which": true,
	"gtk-launch": true, "snap": true, "flatpak": true,
	"head": true, "tail": true, "wc": true, "sort": true, "uniq": true,
	"date": true, "whoami": true, "hostname": true, "uname": true, "pwd": true, "uptime": true,
	"sed": true, "awk": true, "tee": true, "xargs": true,
	"docker": true, "docker-compose": true,
	"go": true, "make": true, "gcc": true, "g++": true,
	"ssh": true, "scp": true, "rsync": true,
	"nano": true, "vim": true,
	"ip": true, "ifconfig": true, "netstat": true, "ss": true, "ping": true,
	"mount": true, "umount": true,
	"crontab": true,
	"diff":    true, "patch": true, "env": true, "true": true,
}

// DangerousCommands — команды, запрещённые для прямого вызова через API.
// Эти команды могут нанести необратимый ущерб системе.
// Ключ — имя команды, значение — причина блокировки.
var DangerousCommands = map[string]string{
	"mkfs":     "форматирование диска запрещено",
	"dd":       "прямая запись на устройства запрещена",
	"shred":    "безвозвратное уничтожение данных запрещено",
	"poweroff": "выключение системы запрещено через API",
	"halt":     "остановка системы запрещена через API",
	"init":     "управление init запрещено",
}

// BlockedPatterns — подстроки, которые запрещены в командах.
// Защита от катастрофических действий: удаление корня, форк-бомбы,
// запись на устройства, выполнение скриптов из интернета и др.
var BlockedPatterns = []string{
	"rm -rf /",
	"rm -rf /*",
	"rm -rf ~",
	":(){ :|:& };:",
	"dd if=/dev/zero of=/dev/sd",
	"dd if=/dev/random of=/dev/sd",
	"mkfs.",
	"curl|bash",
	"curl | bash",
	"curl|sh",
	"curl | sh",
	"wget|bash",
	"wget | bash",
	"> /dev/sd",
	"chmod -R 777 /",
	"chown -R",
	"/etc/shadow",
	"/etc/passwd",
	"nc -l",
	"ncat -l",
	"base64 -d",
	"python -c",
	"python3 -c",
	"perl -e",
	"ruby -e",
}

var subshellRe = regexp.MustCompile("(`[^`]+`|\\$\\([^)]+\\))")

// Result — результат выполнения команды.
//
// Поля:
//   - Stdout: стандартный вывод команды
//   - Stderr: стандартный вывод ошибок
//   - ReturnCode: код возврата (0 = успех)
//   - Error: текст ошибки (пусто при успехе)
type Result struct {
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	ReturnCode int    `json:"returncode"`
	Error      string `json:"error,omitempty"`
}

// CheckCommand — проверяет команду на безопасность без выполнения.
// Возвращает список подкоманд и ошибку, если команда заблокирована.
func CheckCommand(command string) ([]string, error) {
	cmdLower := strings.ToLower(strings.TrimSpace(command))

	if subshellRe.MatchString(command) {
		return nil, fmt.Errorf("подстановка команд (backtick/$()) запрещена")
	}

	for _, pattern := range BlockedPatterns {
		if strings.Contains(cmdLower, pattern) {
			return nil, fmt.Errorf("command contains blocked pattern: %s", pattern)
		}
	}

	subCommands := extractSubCommands(command)
	if len(subCommands) == 0 {
		return nil, fmt.Errorf("empty command")
	}
	for _, sub := range subCommands {
		if !AllowedCommands[sub] {
			return nil, fmt.Errorf("command not allowed: %s", sub)
		}
		if reason, blocked := DangerousCommands[sub]; blocked {
			return nil, fmt.Errorf("dangerous command blocked: %s — %s", sub, reason)
		}
	}
	return subCommands, nil
}

// ExecuteCommand — выполняет команду через bash -c.
//
// В ADMIN_TRUSTED_MODE: проверки логируются как WARN, но НЕ блокируют выполнение.
// В обычном режиме: полная проверка (whitelist, dangerous, blocked patterns).
//
// Параметр command — строка как в терминале (например, "ls -la && df -h").
func ExecuteCommand(command string) Result {
	trusted := execmode.IsTrusted()
	cmdLower := strings.ToLower(strings.TrimSpace(command))

	if subshellRe.MatchString(command) {
		if trusted {
			slog.Warn("[TRUSTED] Подстановка команды — пропущено", slog.String("команда", command))
		} else {
			slog.Warn("Заблокирована подстановка команды", slog.String("команда", command))
			return Result{Error: "подстановка команд (backtick/$()) запрещена"}
		}
	}

	for _, pattern := range BlockedPatterns {
		if strings.Contains(cmdLower, pattern) {
			if trusted {
				slog.Warn("[TRUSTED] Опасный паттерн — пропущено", slog.String("паттерн", pattern), slog.String("команда", command))
			} else {
				slog.Warn("Заблокирован опасный паттерн", slog.String("паттерн", pattern), slog.String("команда", command))
				return Result{Error: "command contains blocked pattern: " + pattern}
			}
		}
	}

	subCommands := extractSubCommands(command)
	if len(subCommands) == 0 {
		return Result{Error: "empty command"}
	}
	for _, sub := range subCommands {
		if !AllowedCommands[sub] {
			if trusted {
				slog.Warn("[TRUSTED] Команда не в белом списке — пропущено", slog.String("команда", sub))
			} else {
				slog.Warn("Команда не в белом списке", slog.String("команда", sub))
				return Result{Error: "command not allowed: " + sub}
			}
		}
		if reason, blocked := DangerousCommands[sub]; blocked {
			if trusted {
				slog.Warn("[TRUSTED] Опасная команда — пропущено", slog.String("команда", sub), slog.String("причина", reason))
			} else {
				slog.Warn("Опасная команда заблокирована", slog.String("команда", sub), slog.String("причина", reason))
				return Result{Error: "dangerous command blocked: " + sub + " — " + reason}
			}
		}
	}

	slog.Info("Выполнение команды", slog.String("команда", command), slog.String("режим", execmode.String()))
	cmd := exec.Command("bash", "-c", command)

	stdout, err := cmd.Output()
	stderr := ""
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr = string(exitErr.Stderr)
		}
		exitCode := -1
		if cmd.ProcessState != nil {
			exitCode = cmd.ProcessState.ExitCode()
		}
		return Result{
			Stdout:     string(stdout),
			Stderr:     stderr,
			ReturnCode: exitCode,
			Error:      err.Error(),
		}
	}

	return Result{
		Stdout:     string(stdout),
		Stderr:     stderr,
		ReturnCode: 0,
	}
}
