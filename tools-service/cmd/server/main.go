package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/neo-2022/openclaw-memory/tools-service/internal/executor"
)

type ExecuteRequest struct {
	Command string `json:"command"`
}

type ExecuteResponse struct {
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	ReturnCode int    `json:"returncode"`
	Error      string `json:"error,omitempty"`
}

type ReadFileRequest struct {
	Path string `json:"path"`
}

type WriteFileRequest struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type ListDirRequest struct {
	Path string `json:"path"`
}

type DeleteFileRequest struct {
	Path string `json:"path"`
}

type FindAppRequest struct {
	Name string `json:"name"`
}

type LaunchAppRequest struct {
	DesktopFile string `json:"desktop_file"`
}

type AddAutostartRequest struct {
	AppName string `json:"app_name"`
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok","service":"tools-service"}`))
}

func executeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req ExecuteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("[TOOLS] execute: ошибка парсинга JSON: %v", err)
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	log.Printf("[TOOLS] execute: команда=%q", req.Command)
	result := executor.ExecuteCommand(req.Command)
	log.Printf("[TOOLS] execute: код=%d, stdout=%d байт, stderr=%d байт, error=%q", result.ReturnCode, len(result.Stdout), len(result.Stderr), result.Error)
	resp := ExecuteResponse{
		Stdout:     result.Stdout,
		Stderr:     result.Stderr,
		ReturnCode: result.ReturnCode,
		Error:      result.Error,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func readFileHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req ReadFileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("[TOOLS] read: ошибка парсинга JSON: %v", err)
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	log.Printf("[TOOLS] read: path=%q", req.Path)
	content, err := executor.ReadFile(req.Path)
	if err != nil {
		log.Printf("[TOOLS] read: ОШИБКА чтения %q: %v", req.Path, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Printf("[TOOLS] read: OK, %d байт из %q", len(content), req.Path)
	json.NewEncoder(w).Encode(map[string]string{"content": content})
}

func writeFileHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req WriteFileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("[TOOLS] write: ошибка парсинга JSON: %v", err)
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	log.Printf("[TOOLS] write: path=%q, content=%d байт", req.Path, len(req.Content))
	err := executor.WriteFile(req.Path, req.Content)
	if err != nil {
		log.Printf("[TOOLS] write: ОШИБКА записи %q: %v", req.Path, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Printf("[TOOLS] write: OK, записано в %q", req.Path)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func listDirHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req ListDirRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("[TOOLS] list: ошибка парсинга JSON: %v", err)
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	log.Printf("[TOOLS] list: path=%q", req.Path)
	files, err := executor.ListDirectory(req.Path)
	if err != nil {
		log.Printf("[TOOLS] list: ОШИБКА чтения директории %q: %v", req.Path, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Printf("[TOOLS] list: OK, %d файлов в %q", len(files), req.Path)
	json.NewEncoder(w).Encode(map[string][]string{"files": files})
}

func deleteFileHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req DeleteFileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("[TOOLS] delete: ошибка парсинга JSON: %v", err)
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	log.Printf("[TOOLS] delete: path=%q", req.Path)
	err := executor.DeleteFile(req.Path)
	if err != nil {
		log.Printf("[TOOLS] delete: ОШИБКА удаления %q: %v", req.Path, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Printf("[TOOLS] delete: OK, удалён %q", req.Path)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func systemInfoHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("[TOOLS] sysinfo: запрос информации о системе")
	info := executor.GetSystemInfo()
	log.Printf("[TOOLS] sysinfo: OS=%s, Arch=%s, Host=%s, Home=%s, User=%s", info.OS, info.Arch, info.Hostname, info.HomeDir, info.User)
	json.NewEncoder(w).Encode(info)
}

func cpuInfoHandler(w http.ResponseWriter, r *http.Request) {
	data, err := executor.GetCPUInfo()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"cpuinfo": data})
}

func memInfoHandler(w http.ResponseWriter, r *http.Request) {
	data, err := executor.GetMemInfo()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"meminfo": data})
}

func cpuTemperatureHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("[TOOLS] cputemp: запрос температуры CPU")
	data, err := executor.GetCPUTemperature()
	if err != nil {
		log.Printf("[TOOLS] cputemp: ОШИБКА: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Printf("[TOOLS] cputemp: OK, %d байт данных", len(data))
	json.NewEncoder(w).Encode(map[string]string{"temperature": data})
}

func systemLoadHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("[TOOLS] sysload: запрос загрузки системы")
	data, err := executor.GetSystemLoad()
	if err != nil {
		log.Printf("[TOOLS] sysload: ОШИБКА: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Printf("[TOOLS] sysload: OK, данные получены")
	json.NewEncoder(w).Encode(data)
}

func findAppHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req FindAppRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("[TOOLS] findapp: ошибка парсинга JSON: %v", err)
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	log.Printf("[TOOLS] findapp: поиск приложения %q", req.Name)
	apps, err := executor.FindApplication(req.Name)
	if err != nil {
		log.Printf("[TOOLS] findapp: ОШИБКА поиска %q: %v", req.Name, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Printf("[TOOLS] findapp: найдено %d приложений для %q", len(apps), req.Name)
	json.NewEncoder(w).Encode(map[string]interface{}{"found": apps})
}

func launchAppHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req LaunchAppRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("[TOOLS] launchapp: ошибка парсинга JSON: %v", err)
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	log.Printf("[TOOLS] launchapp: запуск %q", req.DesktopFile)
	err := executor.LaunchApplication(req.DesktopFile)
	if err != nil {
		log.Printf("[TOOLS] launchapp: ОШИБКА запуска %q: %v", req.DesktopFile, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Printf("[TOOLS] launchapp: OK, запущено %q", req.DesktopFile)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func addAutostartHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req AddAutostartRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	// сначала ищем приложение
	apps, err := executor.FindApplication(req.AppName)
	if err != nil || len(apps) == 0 {
		http.Error(w, "Application not found", http.StatusNotFound)
		return
	}
	// берём первое
	err = executor.AddToAutostart(apps[0].DesktopPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// ============================================================================
// HTTP-обработчики для Яндекс.Диска (REST API)
// ============================================================================

// getYandexDiskClient — создаёт клиент Яндекс.Диска из переменной окружения.
// Сначала проверяет YANDEX_DISK_TOKEN, затем Yandex_Disk.
func getYandexDiskClient() (*executor.YandexDiskClient, error) {
	token := os.Getenv("YANDEX_DISK_TOKEN")
	if token == "" {
		token = os.Getenv("Yandex_Disk")
	}
	if token == "" {
		return nil, fmt.Errorf("токен Яндекс.Диска не настроен (YANDEX_DISK_TOKEN или Yandex_Disk)")
	}
	return executor.NewYandexDiskClient(token), nil
}

// ydiskInfoHandler — возвращает информацию о Яндекс.Диске пользователя.
// GET /ydisk/info
func ydiskInfoHandler(w http.ResponseWriter, r *http.Request) {
	client, err := getYandexDiskClient()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	info, err := client.GetDiskInfo()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

// ydiskListHandler — возвращает содержимое папки на Яндекс.Диске.
// GET /ydisk/list?path=/&limit=20&offset=0
func ydiskListHandler(w http.ResponseWriter, r *http.Request) {
	client, err := getYandexDiskClient()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	path := r.URL.Query().Get("path")
	if path == "" {
		path = "/"
	}
	resource, err := client.ListDir(path, 100, 0)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var items []executor.SimpleDiskItem
	if resource.Embedded != nil {
		items = executor.ToSimpleItems(resource.Embedded.Items)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"path":  path,
		"items": items,
		"total": len(items),
	})
}

// ydiskDownloadHandler — скачивает файл с Яндекс.Диска.
// GET /ydisk/download?path=/Documents/file.txt
func ydiskDownloadHandler(w http.ResponseWriter, r *http.Request) {
	client, err := getYandexDiskClient()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	path := r.URL.Query().Get("path")
	if path == "" {
		http.Error(w, "параметр path обязателен", http.StatusBadRequest)
		return
	}
	data, err := client.DownloadFile(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+path[strings.LastIndex(path, "/")+1:]+"\"")
	w.Write(data)
}

// YdiskUploadRequest — запрос на загрузку файла на Яндекс.Диск.
type YdiskUploadRequest struct {
	Path      string `json:"path"`      // Путь назначения на диске
	Content   string `json:"content"`   // Содержимое файла (base64 или текст)
	Overwrite bool   `json:"overwrite"` // Перезаписать если существует
}

// ydiskUploadHandler — загружает файл на Яндекс.Диск.
// POST /ydisk/upload {"path":"/Documents/file.txt","content":"..."}
func ydiskUploadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	client, err := getYandexDiskClient()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	var req YdiskUploadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	err = client.UploadFile(req.Path, strings.NewReader(req.Content), req.Overwrite)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "path": req.Path})
}

// YdiskCreateDirRequest — запрос на создание папки.
type YdiskCreateDirRequest struct {
	Path string `json:"path"` // Путь создаваемой папки
}

// ydiskCreateDirHandler — создаёт папку на Яндекс.Диске.
// POST /ydisk/mkdir {"path":"/Projects/NewProject"}
func ydiskCreateDirHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	client, err := getYandexDiskClient()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	var req YdiskCreateDirRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	err = client.CreateDir(req.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "path": req.Path})
}

// YdiskDeleteRequest — запрос на удаление файла/папки.
type YdiskDeleteRequest struct {
	Path        string `json:"path"`        // Путь к удаляемому ресурсу
	Permanently bool   `json:"permanently"` // Удалить безвозвратно
}

// ydiskDeleteHandler — удаляет файл или папку с Яндекс.Диска.
// POST /ydisk/delete {"path":"/old_file.txt","permanently":false}
func ydiskDeleteHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	client, err := getYandexDiskClient()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	var req YdiskDeleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	err = client.Delete(req.Path, req.Permanently)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// YdiskMoveRequest — запрос на перемещение/переименование.
type YdiskMoveRequest struct {
	From      string `json:"from"`      // Исходный путь
	To        string `json:"to"`        // Путь назначения
	Overwrite bool   `json:"overwrite"` // Перезаписать если существует
}

// ydiskMoveHandler — перемещает файл/папку на Яндекс.Диске.
// POST /ydisk/move {"from":"/old.txt","to":"/new.txt"}
func ydiskMoveHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	client, err := getYandexDiskClient()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	var req YdiskMoveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	err = client.Move(req.From, req.To, req.Overwrite)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// ydiskSearchHandler — поиск файлов на Яндекс.Диске по типу медиа.
// GET /ydisk/search?media_type=document&limit=20
// ============================================================================
// HTTP-обработчики для взаимодействия с браузером (Админ)
// ============================================================================

// OpenURLRequest — запрос на открытие URL в браузере пользователя.
type OpenURLRequest struct {
	URL string `json:"url"` // URL для открытия
}

// openURLHandler — открывает URL в браузере пользователя через xdg-open.
// POST /browser/open {"url":"https://chat.openai.com"}
func openURLHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req OpenURLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	err := executor.OpenURL(req.URL)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "url": req.URL})
}

// FetchURLRequest — запрос на получение содержимого веб-страницы.
type FetchURLRequest struct {
	URL string `json:"url"` // URL страницы
}

// fetchURLHandler — получает текстовое содержимое веб-страницы.
// POST /browser/fetch {"url":"https://example.com"}
func fetchURLHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req FetchURLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	statusCode, body, err := executor.FetchURL(req.URL)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status_code": statusCode,
		"body":        body,
		"url":         req.URL,
	})
}

// SendToAIChatRequest — запрос на отправку сообщения в AI-чат.
type SendToAIChatRequest struct {
	URL         string `json:"url"`          // URL API эндпоинта AI-чата
	Payload     string `json:"payload"`      // Тело запроса (JSON)
	ContentType string `json:"content_type"` // Тип содержимого (по умолчанию application/json)
}

// sendToAIChatHandler — отправляет POST-запрос к AI-чату.
// POST /browser/ai-chat {"url":"...","payload":"..."}
func sendToAIChatHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req SendToAIChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	statusCode, body, err := executor.SendToAIChat(req.URL, req.Payload, req.ContentType)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status_code": statusCode,
		"body":        body,
		"url":         req.URL,
	})
}

func ydiskSearchHandler(w http.ResponseWriter, r *http.Request) {
	client, err := getYandexDiskClient()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	mediaType := r.URL.Query().Get("media_type")
	results, err := client.Search(mediaType, 50, 0)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	items := executor.ToSimpleItems(results)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"items": items,
		"total": len(items),
	})
}

func main() {
	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/execute", executeHandler)
	http.HandleFunc("/read", readFileHandler)
	http.HandleFunc("/write", writeFileHandler)
	http.HandleFunc("/list", listDirHandler)
	http.HandleFunc("/delete", deleteFileHandler)
	http.HandleFunc("/sysinfo", systemInfoHandler)
	http.HandleFunc("/cpuinfo", cpuInfoHandler)
	http.HandleFunc("/meminfo", memInfoHandler)
	http.HandleFunc("/cputemp", cpuTemperatureHandler)
	http.HandleFunc("/sysload", systemLoadHandler)
	http.HandleFunc("/findapp", findAppHandler)
	http.HandleFunc("/launchapp", launchAppHandler)
	http.HandleFunc("/addautostart", addAutostartHandler)

	// Яндекс.Диск — облачное хранилище (REST API)
	http.HandleFunc("/ydisk/info", ydiskInfoHandler)
	http.HandleFunc("/ydisk/list", ydiskListHandler)
	http.HandleFunc("/ydisk/download", ydiskDownloadHandler)
	http.HandleFunc("/ydisk/upload", ydiskUploadHandler)
	http.HandleFunc("/ydisk/mkdir", ydiskCreateDirHandler)
	http.HandleFunc("/ydisk/delete", ydiskDeleteHandler)
	http.HandleFunc("/ydisk/move", ydiskMoveHandler)
	http.HandleFunc("/ydisk/search", ydiskSearchHandler)

	// Браузер — взаимодействие с браузером и AI-чатами (для Админа)
	http.HandleFunc("/browser/open", openURLHandler)
	http.HandleFunc("/browser/fetch", fetchURLHandler)
	http.HandleFunc("/browser/ai-chat", sendToAIChatHandler)

	log.Println("Tools service starting on :8082")
	log.Fatal(http.ListenAndServe(":8082", nil))
}
