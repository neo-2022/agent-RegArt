package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/neo-2022/openclaw-memory/agent-service/internal/intent"
)

func getToolsBaseURL() string {
	if url := os.Getenv("GATEWAY_URL"); url != "" {
		return url
	}
	return "http://localhost:8080"
}

// HandleIntent вызывает соответствующий обработчик для данного интента
func HandleIntent(intentType string, params intent.Params) (string, error) {
	switch intentType {
	case intent.IntentRememberFact:
		return handleRememberFact(params)
	case intent.IntentAddSynonym:
		return handleAddSynonym(params)
	case intent.IntentAddToAutostart:
		return handleAddToAutostart(params)
	case intent.IntentOpenApp:
		return handleOpenApp(params)
	case intent.IntentOpenFolder:
		return handleOpenFolder(params)
	case intent.IntentHardwareInfo:
		return handleHardwareInfo()
	default:
		return "", fmt.Errorf("unknown intent: %s", intentType)
	}
}

// handleRememberFact отправляет факт в memory-service
func handleRememberFact(params intent.Params) (string, error) {
	fact := params["fact"]
	if fact == "" {
		return "", fmt.Errorf("fact is empty")
	}

	url := getToolsBaseURL() + "/memory/facts"
	payload := map[string]interface{}{
		"text": fact,
		"metadata": map[string]string{
			"source": "user",
		},
	}
	data, _ := json.Marshal(payload)
	resp, err := http.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("failed to call memory-service: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("memory-service returned status %d", resp.StatusCode)
	}

	return fmt.Sprintf("Запомнил: %s", fact), nil
}

// handleAddSynonym добавляет синоним для приложения
func handleAddSynonym(params intent.Params) (string, error) {
	wrong := params["wrong"]
	right := params["right"]
	if wrong == "" || right == "" {
		return "", fmt.Errorf("wrong or right is empty")
	}

	url := getToolsBaseURL() + "/memory/facts"
	payload := map[string]interface{}{
		"text": fmt.Sprintf("Синоним: '%s' соответствует '%s'", wrong, right),
		"metadata": map[string]string{
			"type":   "synonym",
			"wrong":  wrong,
			"right":  right,
			"source": "user",
		},
	}
	data, _ := json.Marshal(payload)
	resp, err := http.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("failed to save synonym: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("memory-service returned status %d", resp.StatusCode)
	}

	return fmt.Sprintf("Синоним добавлен: '%s' теперь соответствует '%s'", wrong, right), nil
}

// handleAddToAutostart добавляет приложение в автозагрузку
func handleAddToAutostart(params intent.Params) (string, error) {
	app := params["app"]
	if app == "" {
		return "", fmt.Errorf("app name is empty")
	}

	// Вызываем tools-service для поиска приложения и добавления в автозагрузку
	// Сначала ищем приложение
	findURL := getToolsBaseURL() + "/tools/findapp"
	findPayload := map[string]string{"name": app}
	data, _ := json.Marshal(findPayload)
	resp, err := http.Post(findURL, "application/json", bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("failed to call tools-service: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("tools-service returned status %d", resp.StatusCode)
	}

	var findResult map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&findResult); err != nil {
		return "", fmt.Errorf("failed to decode find result: %w", err)
	}

	found, ok := findResult["found"].([]interface{})
	if !ok || len(found) == 0 {
		return fmt.Sprintf("Приложение '%s' не найдено", app), nil
	}

	if len(found) == 1 {
		appInfo := found[0].(map[string]interface{})
		desktopPath, ok := appInfo["desktop_path"].(string)
		if !ok || desktopPath == "" {
			return fmt.Sprintf("Для приложения '%s' нет .desktop файла", app), nil
		}
		autostartDir := os.ExpandEnv("$HOME/.config/autostart")
		if err := os.MkdirAll(autostartDir, 0755); err != nil {
			return "", fmt.Errorf("failed to create autostart dir: %w", err)
		}
		dest := autostartDir + "/" + desktopPath[strings.LastIndex(desktopPath, "/")+1:]
		cmd := exec.Command("cp", desktopPath, dest)
		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("failed to copy desktop file: %w", err)
		}
		return fmt.Sprintf("Приложение '%s' добавлено в автозагрузку", app), nil
	}

	var names []string
	for _, item := range found {
		if m, ok := item.(map[string]interface{}); ok {
			if name, ok := m["display_name"].(string); ok {
				names = append(names, name)
			}
		}
	}
	return fmt.Sprintf("Найдено несколько приложений: %s. Уточните, какое именно.", strings.Join(names, ", ")), nil
}

// handleOpenApp открывает приложение
func handleOpenApp(params intent.Params) (string, error) {
	app := params["app"]
	if app == "" {
		return "", fmt.Errorf("app name is empty")
	}

	findURL := getToolsBaseURL() + "/tools/findapp"
	findPayload := map[string]string{"name": app}
	data, _ := json.Marshal(findPayload)
	resp, err := http.Post(findURL, "application/json", bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("failed to call tools-service: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("tools-service returned status %d", resp.StatusCode)
	}

	var findResult map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&findResult); err != nil {
		return "", fmt.Errorf("failed to decode find result: %w", err)
	}

	found, ok := findResult["found"].([]interface{})
	if !ok || len(found) == 0 {
		return fmt.Sprintf("Приложение '%s' не найдено", app), nil
	}

	if len(found) == 1 {
		appInfo := found[0].(map[string]interface{})
		launchURL := getToolsBaseURL() + "/tools/launchapp"
		launchPayload := map[string]string{"desktop_file": appInfo["desktop_path"].(string)}
		launchData, _ := json.Marshal(launchPayload)
		launchResp, err := http.Post(launchURL, "application/json", bytes.NewReader(launchData))
		if err != nil {
			return "", fmt.Errorf("failed to launch: %w", err)
		}
		defer launchResp.Body.Close()
		if launchResp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("launch failed with status %d", launchResp.StatusCode)
		}
		return fmt.Sprintf("Запущено приложение: %s", appInfo["display_name"]), nil
	}

	// Множественный выбор
	var names []string
	for _, item := range found {
		if m, ok := item.(map[string]interface{}); ok {
			if name, ok := m["display_name"].(string); ok {
				names = append(names, name)
			}
		}
	}
	return fmt.Sprintf("Найдено несколько приложений: %s. Уточните, какое именно.", strings.Join(names, ", ")), nil
}

// handleOpenFolder открывает папку
func handleOpenFolder(params intent.Params) (string, error) {
	folderType := params["folder"]
	var path string
	switch folderType {
	case "downloads":
		path = os.ExpandEnv("$HOME/Загрузки")
	case "autostart":
		path = os.ExpandEnv("$HOME/.config/autostart")
	case "home":
		path = os.ExpandEnv("$HOME")
	case "root":
		path = "/"
	default:
		path = os.ExpandEnv("$HOME")
	}

	url := getToolsBaseURL() + "/tools/execute"
	payload := map[string]string{"command": "xdg-open " + path}
	data, _ := json.Marshal(payload)
	resp, err := http.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("failed to call tools-service: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("open_folder returned status %d", resp.StatusCode)
	}

	return fmt.Sprintf("Папка %s открыта", path), nil
}

// handleHardwareInfo получает информацию о железе
func handleHardwareInfo() (string, error) {
	url := getToolsBaseURL() + "/tools/sysinfo"
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to call tools-service: %w", err)
	}
	defer resp.Body.Close()

	var info map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return "", fmt.Errorf("failed to decode sysinfo: %w", err)
	}

	// Формируем читаемый ответ
	return fmt.Sprintf("Информация о системе: ОС %s, архитектура %s, hostname %s", info["os"], info["arch"], info["hostname"]), nil
}
