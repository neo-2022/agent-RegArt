package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/neo-2022/openclaw-memory/tools-service/internal/executor"
	"github.com/neo-2022/openclaw-memory/tools-service/internal/logger"
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
	cid := r.Header.Get("X-Request-ID")
	ctx := logger.WithCorrelationID(r.Context(), cid)
	var req ExecuteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.С(ctx).Error("Ошибка парсинга JSON", slog.String("обработчик", "execute"), slog.String("ошибка", err.Error()))
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	logger.С(ctx).Info("Выполнение команды", slog.String("команда", req.Command))
	result := executor.ExecuteCommand(req.Command)
	logger.С(ctx).Info("Результат выполнения", slog.Int("код", result.ReturnCode), slog.Int("stdout_байт", len(result.Stdout)), slog.Int("stderr_байт", len(result.Stderr)))
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
	cid := r.Header.Get("X-Request-ID")
	ctx := logger.WithCorrelationID(r.Context(), cid)
	var req ReadFileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.С(ctx).Error("Ошибка парсинга JSON", slog.String("обработчик", "read"), slog.String("ошибка", err.Error()))
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	logger.С(ctx).Info("Чтение файла", slog.String("путь", req.Path))
	content, err := executor.ReadFile(req.Path)
	if err != nil {
		logger.С(ctx).Error("Ошибка чтения файла", slog.String("путь", req.Path), slog.String("ошибка", err.Error()))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	logger.С(ctx).Info("Файл прочитан", slog.Int("байт", len(content)), slog.String("путь", req.Path))
	json.NewEncoder(w).Encode(map[string]string{"content": content})
}

func writeFileHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cid := r.Header.Get("X-Request-ID")
	ctx := logger.WithCorrelationID(r.Context(), cid)
	var req WriteFileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.С(ctx).Error("Ошибка парсинга JSON", slog.String("обработчик", "write"), slog.String("ошибка", err.Error()))
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	logger.С(ctx).Info("Запись файла", slog.String("путь", req.Path), slog.Int("байт", len(req.Content)))
	err := executor.WriteFile(req.Path, req.Content)
	if err != nil {
		logger.С(ctx).Error("Ошибка записи файла", slog.String("путь", req.Path), slog.String("ошибка", err.Error()))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	logger.С(ctx).Info("Файл записан", slog.String("путь", req.Path))
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func listDirHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cid := r.Header.Get("X-Request-ID")
	ctx := logger.WithCorrelationID(r.Context(), cid)
	var req ListDirRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.С(ctx).Error("Ошибка парсинга JSON", slog.String("обработчик", "list"), slog.String("ошибка", err.Error()))
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	logger.С(ctx).Info("Листинг директории", slog.String("путь", req.Path))
	files, err := executor.ListDirectory(req.Path)
	if err != nil {
		logger.С(ctx).Error("Ошибка чтения директории", slog.String("путь", req.Path), slog.String("ошибка", err.Error()))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	logger.С(ctx).Info("Директория прочитана", slog.Int("файлов", len(files)), slog.String("путь", req.Path))
	json.NewEncoder(w).Encode(map[string][]string{"files": files})
}

func deleteFileHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cid := r.Header.Get("X-Request-ID")
	ctx := logger.WithCorrelationID(r.Context(), cid)
	var req DeleteFileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.С(ctx).Error("Ошибка парсинга JSON", slog.String("обработчик", "delete"), slog.String("ошибка", err.Error()))
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	logger.С(ctx).Info("Удаление файла", slog.String("путь", req.Path))
	err := executor.DeleteFile(req.Path)
	if err != nil {
		logger.С(ctx).Error("Ошибка удаления файла", slog.String("путь", req.Path), slog.String("ошибка", err.Error()))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	logger.С(ctx).Info("Файл удалён", slog.String("путь", req.Path))
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func systemInfoHandler(w http.ResponseWriter, r *http.Request) {
	cid := r.Header.Get("X-Request-ID")
	ctx := logger.WithCorrelationID(r.Context(), cid)
	logger.С(ctx).Info("Запрос информации о системе")
	info := executor.GetSystemInfo()
	logger.С(ctx).Info("Информация о системе", slog.String("os", info.OS), slog.String("arch", info.Arch), slog.String("host", info.Hostname))
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
	cid := r.Header.Get("X-Request-ID")
	ctx := logger.WithCorrelationID(r.Context(), cid)
	logger.С(ctx).Info("Запрос температуры CPU")
	data, err := executor.GetCPUTemperature()
	if err != nil {
		logger.С(ctx).Error("Ошибка получения температуры", slog.String("ошибка", err.Error()))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	logger.С(ctx).Info("Температура получена", slog.Int("байт", len(data)))
	json.NewEncoder(w).Encode(map[string]string{"temperature": data})
}

func systemLoadHandler(w http.ResponseWriter, r *http.Request) {
	cid := r.Header.Get("X-Request-ID")
	ctx := logger.WithCorrelationID(r.Context(), cid)
	logger.С(ctx).Info("Запрос загрузки системы")
	data, err := executor.GetSystemLoad()
	if err != nil {
		logger.С(ctx).Error("Ошибка получения загрузки", slog.String("ошибка", err.Error()))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	logger.С(ctx).Info("Загрузка системы получена")
	json.NewEncoder(w).Encode(data)
}

func findAppHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cid := r.Header.Get("X-Request-ID")
	ctx := logger.WithCorrelationID(r.Context(), cid)
	var req FindAppRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.С(ctx).Error("Ошибка парсинга JSON", slog.String("обработчик", "findapp"), slog.String("ошибка", err.Error()))
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	logger.С(ctx).Info("Поиск приложения", slog.String("имя", req.Name))
	apps, err := executor.FindApplication(req.Name)
	if err != nil {
		logger.С(ctx).Error("Ошибка поиска приложения", slog.String("имя", req.Name), slog.String("ошибка", err.Error()))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	logger.С(ctx).Info("Приложения найдены", slog.Int("количество", len(apps)), slog.String("имя", req.Name))
	json.NewEncoder(w).Encode(map[string]interface{}{"found": apps})
}

func launchAppHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cid := r.Header.Get("X-Request-ID")
	ctx := logger.WithCorrelationID(r.Context(), cid)
	var req LaunchAppRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.С(ctx).Error("Ошибка парсинга JSON", slog.String("обработчик", "launchapp"), slog.String("ошибка", err.Error()))
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	logger.С(ctx).Info("Запуск приложения", slog.String("файл", req.DesktopFile))
	err := executor.LaunchApplication(req.DesktopFile)
	if err != nil {
		logger.С(ctx).Error("Ошибка запуска приложения", slog.String("файл", req.DesktopFile), slog.String("ошибка", err.Error()))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	logger.С(ctx).Info("Приложение запущено", slog.String("файл", req.DesktopFile))
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
	logger.Init("tools-service")

	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/execute", executeHandler)
	mux.HandleFunc("/read", readFileHandler)
	mux.HandleFunc("/write", writeFileHandler)
	mux.HandleFunc("/list", listDirHandler)
	mux.HandleFunc("/delete", deleteFileHandler)
	mux.HandleFunc("/sysinfo", systemInfoHandler)
	mux.HandleFunc("/cpuinfo", cpuInfoHandler)
	mux.HandleFunc("/meminfo", memInfoHandler)
	mux.HandleFunc("/cputemp", cpuTemperatureHandler)
	mux.HandleFunc("/sysload", systemLoadHandler)
	mux.HandleFunc("/findapp", findAppHandler)
	mux.HandleFunc("/launchapp", launchAppHandler)
	mux.HandleFunc("/addautostart", addAutostartHandler)

	mux.HandleFunc("/ydisk/info", ydiskInfoHandler)
	mux.HandleFunc("/ydisk/list", ydiskListHandler)
	mux.HandleFunc("/ydisk/download", ydiskDownloadHandler)
	mux.HandleFunc("/ydisk/upload", ydiskUploadHandler)
	mux.HandleFunc("/ydisk/mkdir", ydiskCreateDirHandler)
	mux.HandleFunc("/ydisk/delete", ydiskDeleteHandler)
	mux.HandleFunc("/ydisk/move", ydiskMoveHandler)
	mux.HandleFunc("/ydisk/search", ydiskSearchHandler)

	mux.HandleFunc("/browser/open", openURLHandler)
	mux.HandleFunc("/browser/fetch", fetchURLHandler)
	mux.HandleFunc("/browser/ai-chat", sendToAIChatHandler)

	port := os.Getenv("TOOLS_PORT")
	if port == "" {
		port = "8082"
	}

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		slog.Info("Tools-service запускается", slog.String("порт", port))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Ошибка сервера", slog.String("ошибка", err.Error()))
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	slog.Info("Получен сигнал завершения", slog.String("сигнал", sig.String()))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("Ошибка при завершении сервера", slog.String("ошибка", err.Error()))
	}
	slog.Info("Сервер корректно остановлен")
}
