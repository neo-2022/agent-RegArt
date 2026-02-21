package executor

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// AppInfo содержит информацию о найденном приложении.
type AppInfo struct {
	DisplayName string `json:"display_name"`
	DesktopPath string `json:"desktop_path"`
	Comment     string `json:"comment"`
	ExecCmd     string `json:"exec_cmd"`
}

// FindApplication ищет .desktop файлы по имени.
func FindApplication(name string) ([]AppInfo, error) {
	// Директории, где искать .desktop файлы
	dirs := []string{
		"/usr/share/applications",
		os.ExpandEnv("$HOME/.local/share/applications"),
		"/var/lib/flatpak/exports/share/applications",
		os.ExpandEnv("$HOME/.local/share/flatpak/exports/share/applications"),
		"/var/lib/snapd/desktop/applications",
		"/usr/local/share/applications",
	}

	nameLower := strings.ToLower(name)
	var results []AppInfo

	for _, d := range dirs {
		if _, err := os.Stat(d); os.IsNotExist(err) {
			continue
		}
		files, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		for _, f := range files {
			if !strings.HasSuffix(f.Name(), ".desktop") {
				continue
			}
			path := filepath.Join(d, f.Name())
			content, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			lines := strings.Split(string(content), "\n")
			var displayName, genericName, comment, execCmd string
			for _, line := range lines {
				if strings.HasPrefix(line, "Name=") && displayName == "" {
					displayName = strings.TrimPrefix(line, "Name=")
				}
				if strings.HasPrefix(line, "GenericName=") && genericName == "" {
					genericName = strings.TrimPrefix(line, "GenericName=")
				}
				if strings.HasPrefix(line, "Comment=") && comment == "" {
					comment = strings.TrimPrefix(line, "Comment=")
				}
				if strings.HasPrefix(line, "Exec=") && execCmd == "" {
					execCmd = strings.TrimPrefix(line, "Exec=")
					// убираем параметры %U, %F и т.д.
					execCmd = strings.Split(execCmd, "%")[0]
					execCmd = strings.TrimSpace(execCmd)
				}
			}
			if displayName == "" {
				displayName = genericName
			}
			if displayName == "" {
				continue
			}
			if strings.Contains(strings.ToLower(displayName), nameLower) || strings.Contains(strings.ToLower(execCmd), nameLower) {
				results = append(results, AppInfo{
					DisplayName: displayName,
					DesktopPath: path,
					Comment:     comment,
					ExecCmd:     execCmd,
				})
			}
		}
	}
	return results, nil
}

// LaunchApplication запускает приложение по .desktop файлу или по команде.
func LaunchApplication(desktopPath string) error {
	if _, err := os.Stat(desktopPath); err == nil {
		cmd := exec.Command("gtk-launch", filepath.Base(desktopPath))
		return cmd.Run()
	}
	// если не .desktop, пробуем выполнить как команду
	cmd := exec.Command(desktopPath)
	return cmd.Run()
}

// AddToAutostart добавляет .desktop файл в автозагрузку пользователя.
func AddToAutostart(desktopPath string) error {
	autostartDir := os.ExpandEnv("$HOME/.config/autostart")
	if err := os.MkdirAll(autostartDir, 0755); err != nil {
		return err
	}
	dest := filepath.Join(autostartDir, filepath.Base(desktopPath))
	return copyFile(desktopPath, dest)
}

// copyFile копирует файл.
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}
