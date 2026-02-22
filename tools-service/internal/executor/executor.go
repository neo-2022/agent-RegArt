package executor

import (
	"os/exec"
	"strings"
)

// extractSubCommands — извлекает имена всех команд из составной команды.
// Разбирает цепочки через &&, ||, |, ; и подстановки $(...)
// Возвращает список имён первых слов каждой подкоманды для проверки по белому списку.
func extractSubCommands(command string) []string {
	var cmds []string
	// Разделяем по &&, ||, |, ;
	for _, sep := range []string{"&&", "||", "|", ";"} {
		command = strings.ReplaceAll(command, sep, "\n")
	}
	lines := strings.Split(command, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Пропускаем строки начинающиеся с ( — это подоболочки вроде (crontab -l ...)
		line = strings.TrimLeft(line, "( ")
		parts := strings.Fields(line)
		if len(parts) > 0 {
			cmds = append(cmds, parts[0])
		}
	}
	return cmds
}

// AllowedCommands — белый список разрешённых команд (первые слова).
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
	"date": true, "whoami": true, "hostname": true, "uname": true,
	"sed": true, "awk": true, "tee": true, "xargs": true,
	"docker": true, "docker-compose": true,
	"go": true, "make": true, "gcc": true, "g++": true,
	"ssh": true, "scp": true, "rsync": true,
	"nano": true, "vim": true,
	"ip": true, "ifconfig": true, "netstat": true, "ss": true, "ping": true,
	"mount": true, "umount": true,
	"crontab": true,
	"diff": true, "patch": true, "env": true, "true": true,
}

// DangerousCommands — команды, запрещённые для прямого вызова через API.
// Эти команды могут нанести необратимый ущерб системе.
var DangerousCommands = map[string]string{
	"mkfs":     "форматирование диска запрещено",
	"dd":       "прямая запись на устройства запрещена",
	"shred":    "безвозвратное уничтожение данных запрещено",
	"poweroff": "выключение системы запрещено через API",
	"halt":     "остановка системы запрещена через API",
	"init":     "управление init запрещено",
}

// BlockedPatterns — подстроки, которые запрещены в командах (защита от катастроф).
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
}

// Result содержит результат выполнения команды.
type Result struct {
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	ReturnCode int    `json:"returncode"`
	Error      string `json:"error,omitempty"`
}

// ExecuteCommand выполняет команду безопасно.
// command должна быть строкой, как в терминале (например, "ls -la").
func ExecuteCommand(command string) Result {
	cmdLower := strings.ToLower(strings.TrimSpace(command))

	// Проверка на запрещённые паттерны
	for _, pattern := range BlockedPatterns {
		if strings.Contains(cmdLower, pattern) {
			return Result{Error: "command contains blocked pattern: " + pattern}
		}
	}

	// Извлечение всех команд из цепочки (разделители: &&, ||, |, ;)
	subCommands := extractSubCommands(command)
	if len(subCommands) == 0 {
		return Result{Error: "empty command"}
	}
	for _, sub := range subCommands {
		if !AllowedCommands[sub] {
			return Result{Error: "command not allowed: " + sub}
		}
		if reason, blocked := DangerousCommands[sub]; blocked {
			return Result{Error: "dangerous command blocked: " + sub + " — " + reason}
		}
	}

	// Выполнение через bash -c для поддержки cd, &&, |, подстановок
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
