// Политики выполнения команд по ролям.
//
// Определяет, какие команды доступны каждой роли:
//   - viewer: только безопасные read-only команды
//   - operator: viewer + файловые операции, пакетные менеджеры
//   - admin: полный AllowedCommands (но с dangerous/pattern checks)
package auth

// ViewerCommands — команды, доступные роли viewer (только чтение).
var ViewerCommands = map[string]bool{
	"ls": true, "cat": true, "grep": true, "find": true,
	"ps": true, "top": true, "free": true, "df": true, "du": true,
	"echo": true, "head": true, "tail": true, "wc": true,
	"sort": true, "uniq": true, "date": true, "whoami": true,
	"hostname": true, "uname": true, "env": true, "true": true,
	"lscpu": true, "lsusb": true, "lspci": true, "lsblk": true,
	"sensors": true, "nvidia-smi": true, "which": true,
	"diff": true, "ip": true, "ifconfig": true, "netstat": true,
	"ss": true, "ping": true,
}

// OperatorCommands — команды, доступные роли operator (viewer + мутации).
var OperatorCommands = map[string]bool{
	"mkdir": true, "rmdir": true, "touch": true, "cp": true, "mv": true,
	"python": true, "python3": true, "pip": true, "pip3": true,
	"node": true, "npm": true, "yarn": true, "git": true,
	"curl": true, "wget": true, "tar": true, "gzip": true,
	"gunzip": true, "zip": true, "unzip": true,
	"apt": true, "apt-get": true, "dnf": true, "yum": true, "pacman": true,
	"dpkg": true, "rpm": true, "snap": true, "flatpak": true,
	"sed": true, "awk": true, "tee": true, "xargs": true,
	"docker": true, "docker-compose": true,
	"go": true, "make": true, "gcc": true, "g++": true,
	"nano": true, "vim": true, "patch": true,
	"ssh": true, "scp": true, "rsync": true,
	"xdg-open": true, "gtk-launch": true,
}

// RoleAllowedCommand — проверяет, разрешена ли команда для данной роли.
// admin имеет доступ ко всем командам из AllowedCommands executor'а.
// operator = viewer + OperatorCommands.
// viewer = только ViewerCommands.
func RoleAllowedCommand(role Role, cmd string) bool {
	switch role {
	case RoleAdmin:
		return true
	case RoleOperator:
		if ViewerCommands[cmd] || OperatorCommands[cmd] {
			return true
		}
		return false
	case RoleViewer:
		return ViewerCommands[cmd]
	default:
		return false
	}
}
