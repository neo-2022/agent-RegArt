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
	"sync/atomic"
	"syscall"
	"time"

	"github.com/neo-2022/openclaw-memory/tools-service/internal/apierror"
	"github.com/neo-2022/openclaw-memory/tools-service/internal/auth"
	"github.com/neo-2022/openclaw-memory/tools-service/internal/execmode"
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

var toolsRequestCounter uint64

func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			n := atomic.AddUint64(&toolsRequestCounter, 1)
			requestID = fmt.Sprintf("tools-%d-%d", time.Now().UnixNano(), n)
		}
		w.Header().Set("X-Request-ID", requestID)
		r.Header.Set("X-Request-ID", requestID)
		next.ServeHTTP(w, r)
	})
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok","service":"tools-service"}`))
}

func executeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		apierror.MethodNotAllowed(w, r.Header.Get("X-Request-ID"))
		return
	}
	cid := r.Header.Get("X-Request-ID")
	ctx := logger.WithCorrelationID(r.Context(), cid)
	var req ExecuteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.С(ctx).Error("Ошибка парсинга JSON", slog.String("обработчик", "execute"), slog.String("ошибка", err.Error()))
		apierror.BadRequest(w, cid, "Невалидный JSON", "Проверьте формат тела запроса")
		return
	}

	role := auth.RoleFromContext(r.Context())
	subCmds, err := executor.CheckCommand(req.Command)
	if err != nil {
		logger.С(ctx).Warn("Команда заблокирована", slog.String("команда", req.Command), slog.String("ошибка", err.Error()))
		apierror.Forbidden(w, cid, err.Error(), "Команда не прошла проверку безопасности")
		return
	}
	for _, sub := range subCmds {
		if !auth.RoleAllowedCommand(role, sub) {
			logger.С(ctx).Warn("Команда запрещена для роли",
				slog.String("роль", string(role)),
				slog.String("команда", sub),
			)
			apierror.Forbidden(w, cid, "команда "+sub+" недоступна для роли "+string(role), "Требуется роль с более высоким уровнем доступа")
			return
		}
	}

	logger.С(ctx).Info("Выполнение команды", slog.String("команда", req.Command), slog.String("роль", string(role)))
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
		apierror.MethodNotAllowed(w, r.Header.Get("X-Request-ID"))
		return
	}
	cid := r.Header.Get("X-Request-ID")
	ctx := logger.WithCorrelationID(r.Context(), cid)
	var req ReadFileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.С(ctx).Error("Ошибка парсинга JSON", slog.String("обработчик", "read"), slog.String("ошибка", err.Error()))
		apierror.BadRequest(w, cid, "Невалидный JSON", "Проверьте формат тела запроса")
		return
	}
	logger.С(ctx).Info("Чтение файла", slog.String("путь", req.Path))
	content, err := executor.ReadFile(req.Path)
	if err != nil {
		logger.С(ctx).Error("Ошибка чтения файла", slog.String("путь", req.Path), slog.String("ошибка", err.Error()))
		apierror.InternalError(w, cid, err.Error(), "Проверьте путь и права доступа")
		return
	}
	logger.С(ctx).Info("Файл прочитан", slog.Int("байт", len(content)), slog.String("путь", req.Path))
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"content": content})
}

func writeFileHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		apierror.MethodNotAllowed(w, r.Header.Get("X-Request-ID"))
		return
	}
	cid := r.Header.Get("X-Request-ID")
	ctx := logger.WithCorrelationID(r.Context(), cid)
	var req WriteFileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.С(ctx).Error("Ошибка парсинга JSON", slog.String("обработчик", "write"), slog.String("ошибка", err.Error()))
		apierror.BadRequest(w, cid, "Невалидный JSON", "Проверьте формат тела запроса")
		return
	}
	logger.С(ctx).Info("Запись файла", slog.String("путь", req.Path), slog.Int("байт", len(req.Content)))
	err := executor.WriteFile(req.Path, req.Content)
	if err != nil {
		logger.С(ctx).Error("Ошибка записи файла", slog.String("путь", req.Path), slog.String("ошибка", err.Error()))
		apierror.InternalError(w, cid, err.Error(), "Проверьте путь и права доступа")
		return
	}
	logger.С(ctx).Info("Файл записан", slog.String("путь", req.Path))
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func listDirHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		apierror.MethodNotAllowed(w, r.Header.Get("X-Request-ID"))
		return
	}
	cid := r.Header.Get("X-Request-ID")
	ctx := logger.WithCorrelationID(r.Context(), cid)
	var req ListDirRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.С(ctx).Error("Ошибка парсинга JSON", slog.String("обработчик", "list"), slog.String("ошибка", err.Error()))
		apierror.BadRequest(w, cid, "Невалидный JSON", "Проверьте формат тела запроса")
		return
	}
	logger.С(ctx).Info("Листинг директории", slog.String("путь", req.Path))
	files, err := executor.ListDirectory(req.Path)
	if err != nil {
		logger.С(ctx).Error("Ошибка чтения директории", slog.String("путь", req.Path), slog.String("ошибка", err.Error()))
		apierror.InternalError(w, cid, err.Error(), "Проверьте путь и права доступа")
		return
	}
	logger.С(ctx).Info("Директория прочитана", slog.Int("файлов", len(files)), slog.String("путь", req.Path))
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string][]string{"files": files})
}

func deleteFileHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		apierror.MethodNotAllowed(w, r.Header.Get("X-Request-ID"))
		return
	}
	cid := r.Header.Get("X-Request-ID")
	ctx := logger.WithCorrelationID(r.Context(), cid)
	var req DeleteFileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.С(ctx).Error("Ошибка парсинга JSON", slog.String("обработчик", "delete"), slog.String("ошибка", err.Error()))
		apierror.BadRequest(w, cid, "Невалидный JSON", "Проверьте формат тела запроса")
		return
	}
	logger.С(ctx).Info("Удаление файла", slog.String("путь", req.Path))
	err := executor.DeleteFile(req.Path)
	if err != nil {
		logger.С(ctx).Error("Ошибка удаления файла", slog.String("путь", req.Path), slog.String("ошибка", err.Error()))
		apierror.InternalError(w, cid, err.Error(), "Проверьте путь и права доступа")
		return
	}
	logger.С(ctx).Info("Файл удалён", slog.String("путь", req.Path))
	w.Header().Set("Content-Type", "application/json")
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
	cid := r.Header.Get("X-Request-ID")
	ctx := logger.WithCorrelationID(r.Context(), cid)
	data, err := executor.GetCPUInfo()
	if err != nil {
		logger.С(ctx).Error("Ошибка получения информации CPU", slog.String("ошибка", err.Error()))
		apierror.InternalError(w, cid, err.Error(), "Попробуйте повторить запрос позже")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"cpuinfo": data})
}

func memInfoHandler(w http.ResponseWriter, r *http.Request) {
	cid := r.Header.Get("X-Request-ID")
	ctx := logger.WithCorrelationID(r.Context(), cid)
	data, err := executor.GetMemInfo()
	if err != nil {
		logger.С(ctx).Error("Ошибка получения информации памяти", slog.String("ошибка", err.Error()))
		apierror.InternalError(w, cid, err.Error(), "Попробуйте повторить запрос позже")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"meminfo": data})
}

func cpuTemperatureHandler(w http.ResponseWriter, r *http.Request) {
	cid := r.Header.Get("X-Request-ID")
	ctx := logger.WithCorrelationID(r.Context(), cid)
	logger.С(ctx).Info("Запрос температуры CPU")
	data, err := executor.GetCPUTemperature()
	if err != nil {
		logger.С(ctx).Error("Ошибка получения температуры", slog.String("ошибка", err.Error()))
		apierror.InternalError(w, cid, err.Error(), "Попробуйте повторить запрос позже")
		return
	}
	logger.С(ctx).Info("Температура получена", slog.Int("байт", len(data)))
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"temperature": data})
}

func systemLoadHandler(w http.ResponseWriter, r *http.Request) {
	cid := r.Header.Get("X-Request-ID")
	ctx := logger.WithCorrelationID(r.Context(), cid)
	logger.С(ctx).Info("Запрос загрузки системы")
	data, err := executor.GetSystemLoad()
	if err != nil {
		logger.С(ctx).Error("Ошибка получения загрузки", slog.String("ошибка", err.Error()))
		apierror.InternalError(w, cid, err.Error(), "Попробуйте повторить запрос позже")
		return
	}
	logger.С(ctx).Info("Загрузка системы получена")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func findAppHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		apierror.MethodNotAllowed(w, r.Header.Get("X-Request-ID"))
		return
	}
	cid := r.Header.Get("X-Request-ID")
	ctx := logger.WithCorrelationID(r.Context(), cid)
	var req FindAppRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.С(ctx).Error("Ошибка парсинга JSON", slog.String("обработчик", "findapp"), slog.String("ошибка", err.Error()))
		apierror.BadRequest(w, cid, "Невалидный JSON", "Проверьте формат тела запроса")
		return
	}
	logger.С(ctx).Info("Поиск приложения", slog.String("имя", req.Name))
	apps, err := executor.FindApplication(req.Name)
	if err != nil {
		logger.С(ctx).Error("Ошибка поиска приложения", slog.String("имя", req.Name), slog.String("ошибка", err.Error()))
		apierror.InternalError(w, cid, err.Error(), "Попробуйте изменить запрос поиска")
		return
	}
	logger.С(ctx).Info("Приложения найдены", slog.Int("количество", len(apps)), slog.String("имя", req.Name))
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"found": apps})
}

func launchAppHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		apierror.MethodNotAllowed(w, r.Header.Get("X-Request-ID"))
		return
	}
	cid := r.Header.Get("X-Request-ID")
	ctx := logger.WithCorrelationID(r.Context(), cid)
	var req LaunchAppRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.С(ctx).Error("Ошибка парсинга JSON", slog.String("обработчик", "launchapp"), slog.String("ошибка", err.Error()))
		apierror.BadRequest(w, cid, "Невалидный JSON", "Проверьте формат тела запроса")
		return
	}
	logger.С(ctx).Info("Запуск приложения", slog.String("файл", req.DesktopFile))
	err := executor.LaunchApplication(req.DesktopFile)
	if err != nil {
		logger.С(ctx).Error("Ошибка запуска приложения", slog.String("файл", req.DesktopFile), slog.String("ошибка", err.Error()))
		apierror.InternalError(w, cid, err.Error(), "Проверьте путь к .desktop файлу")
		return
	}
	logger.С(ctx).Info("Приложение запущено", slog.String("файл", req.DesktopFile))
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func addAutostartHandler(w http.ResponseWriter, r *http.Request) {
	cid := r.Header.Get("X-Request-ID")
	if r.Method != http.MethodPost {
		apierror.MethodNotAllowed(w, cid)
		return
	}
	ctx := logger.WithCorrelationID(r.Context(), cid)
	var req AddAutostartRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.BadRequest(w, cid, "Невалидный JSON", "Проверьте формат тела запроса")
		return
	}
	logger.С(ctx).Info("Добавление в автозапуск", slog.String("приложение", req.AppName))
	apps, err := executor.FindApplication(req.AppName)
	if err != nil || len(apps) == 0 {
		apierror.NotFound(w, cid, "Приложение не найдено")
		return
	}
	err = executor.AddToAutostart(apps[0].DesktopPath)
	if err != nil {
		logger.С(ctx).Error("Ошибка добавления в автозапуск", slog.String("ошибка", err.Error()))
		apierror.InternalError(w, cid, err.Error(), "")
		return
	}
	w.Header().Set("Content-Type", "application/json")
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
	cid := r.Header.Get("X-Request-ID")
	client, err := getYandexDiskClient()
	if err != nil {
		apierror.ServiceUnavailable(w, cid, err.Error(), "Настройте YANDEX_DISK_TOKEN")
		return
	}
	info, err := client.GetDiskInfo()
	if err != nil {
		apierror.InternalError(w, cid, err.Error(), "")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

// ydiskListHandler — возвращает содержимое папки на Яндекс.Диске.
// GET /ydisk/list?path=/&limit=20&offset=0
func ydiskListHandler(w http.ResponseWriter, r *http.Request) {
	cid := r.Header.Get("X-Request-ID")
	client, err := getYandexDiskClient()
	if err != nil {
		apierror.ServiceUnavailable(w, cid, err.Error(), "Настройте YANDEX_DISK_TOKEN")
		return
	}
	path := r.URL.Query().Get("path")
	if path == "" {
		path = "/"
	}
	resource, err := client.ListDir(path, 100, 0)
	if err != nil {
		apierror.InternalError(w, cid, err.Error(), "")
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
	cid := r.Header.Get("X-Request-ID")
	client, err := getYandexDiskClient()
	if err != nil {
		apierror.ServiceUnavailable(w, cid, err.Error(), "Настройте YANDEX_DISK_TOKEN")
		return
	}
	path := r.URL.Query().Get("path")
	if path == "" {
		apierror.BadRequest(w, cid, "параметр path обязателен", "Добавьте ?path=/...")
		return
	}
	data, err := client.DownloadFile(path)
	if err != nil {
		apierror.InternalError(w, cid, err.Error(), "")
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
	cid := r.Header.Get("X-Request-ID")
	if r.Method != http.MethodPost {
		apierror.MethodNotAllowed(w, cid)
		return
	}
	client, err := getYandexDiskClient()
	if err != nil {
		apierror.ServiceUnavailable(w, cid, err.Error(), "Настройте YANDEX_DISK_TOKEN")
		return
	}
	var req YdiskUploadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.BadRequest(w, cid, "Невалидный JSON", "Проверьте формат тела запроса")
		return
	}
	err = client.UploadFile(req.Path, strings.NewReader(req.Content), req.Overwrite)
	if err != nil {
		apierror.InternalError(w, cid, err.Error(), "")
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
	cid := r.Header.Get("X-Request-ID")
	if r.Method != http.MethodPost {
		apierror.MethodNotAllowed(w, cid)
		return
	}
	client, err := getYandexDiskClient()
	if err != nil {
		apierror.ServiceUnavailable(w, cid, err.Error(), "Настройте YANDEX_DISK_TOKEN")
		return
	}
	var req YdiskCreateDirRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.BadRequest(w, cid, "Невалидный JSON", "Проверьте формат тела запроса")
		return
	}
	err = client.CreateDir(req.Path)
	if err != nil {
		apierror.InternalError(w, cid, err.Error(), "")
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
	cid := r.Header.Get("X-Request-ID")
	if r.Method != http.MethodPost {
		apierror.MethodNotAllowed(w, cid)
		return
	}
	client, err := getYandexDiskClient()
	if err != nil {
		apierror.ServiceUnavailable(w, cid, err.Error(), "Настройте YANDEX_DISK_TOKEN")
		return
	}
	var req YdiskDeleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.BadRequest(w, cid, "Невалидный JSON", "Проверьте формат тела запроса")
		return
	}
	err = client.Delete(req.Path, req.Permanently)
	if err != nil {
		apierror.InternalError(w, cid, err.Error(), "")
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
	cid := r.Header.Get("X-Request-ID")
	if r.Method != http.MethodPost {
		apierror.MethodNotAllowed(w, cid)
		return
	}
	client, err := getYandexDiskClient()
	if err != nil {
		apierror.ServiceUnavailable(w, cid, err.Error(), "Настройте YANDEX_DISK_TOKEN")
		return
	}
	var req YdiskMoveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.BadRequest(w, cid, "Невалидный JSON", "Проверьте формат тела запроса")
		return
	}
	err = client.Move(req.From, req.To, req.Overwrite)
	if err != nil {
		apierror.InternalError(w, cid, err.Error(), "")
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
	cid := r.Header.Get("X-Request-ID")
	if r.Method != http.MethodPost {
		apierror.MethodNotAllowed(w, cid)
		return
	}
	var req OpenURLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.BadRequest(w, cid, "Невалидный JSON", "Проверьте формат тела запроса")
		return
	}
	err := executor.OpenURL(req.URL)
	if err != nil {
		apierror.InternalError(w, cid, err.Error(), "")
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
	cid := r.Header.Get("X-Request-ID")
	if r.Method != http.MethodPost {
		apierror.MethodNotAllowed(w, cid)
		return
	}
	var req FetchURLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.BadRequest(w, cid, "Невалидный JSON", "Проверьте формат тела запроса")
		return
	}
	statusCode, body, err := executor.FetchURL(req.URL)
	if err != nil {
		apierror.InternalError(w, cid, err.Error(), "")
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
	cid := r.Header.Get("X-Request-ID")
	if r.Method != http.MethodPost {
		apierror.MethodNotAllowed(w, cid)
		return
	}
	var req SendToAIChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.BadRequest(w, cid, "Невалидный JSON", "Проверьте формат тела запроса")
		return
	}
	statusCode, body, err := executor.SendToAIChat(req.URL, req.Payload, req.ContentType)
	if err != nil {
		apierror.InternalError(w, cid, err.Error(), "")
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
	cid := r.Header.Get("X-Request-ID")
	client, err := getYandexDiskClient()
	if err != nil {
		apierror.ServiceUnavailable(w, cid, err.Error(), "Настройте YANDEX_DISK_TOKEN")
		return
	}
	mediaType := r.URL.Query().Get("media_type")
	results, err := client.Search(mediaType, 50, 0)
	if err != nil {
		apierror.InternalError(w, cid, err.Error(), "")
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
	execmode.Init()

	tokenRoles := auth.LoadTokensFromEnv()

	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthHandler)

	mux.HandleFunc("/execute", auth.WithAuth(auth.RoleAdmin, tokenRoles, executeHandler))
	mux.HandleFunc("/addautostart", auth.WithAuth(auth.RoleAdmin, tokenRoles, addAutostartHandler))

	mux.HandleFunc("/read", auth.WithAuth(auth.RoleViewer, tokenRoles, readFileHandler))
	mux.HandleFunc("/list", auth.WithAuth(auth.RoleViewer, tokenRoles, listDirHandler))
	mux.HandleFunc("/findapp", auth.WithAuth(auth.RoleViewer, tokenRoles, findAppHandler))
	mux.HandleFunc("/sysinfo", auth.WithAuth(auth.RoleViewer, tokenRoles, systemInfoHandler))
	mux.HandleFunc("/cpuinfo", auth.WithAuth(auth.RoleViewer, tokenRoles, cpuInfoHandler))
	mux.HandleFunc("/meminfo", auth.WithAuth(auth.RoleViewer, tokenRoles, memInfoHandler))
	mux.HandleFunc("/cputemp", auth.WithAuth(auth.RoleViewer, tokenRoles, cpuTemperatureHandler))
	mux.HandleFunc("/sysload", auth.WithAuth(auth.RoleViewer, tokenRoles, systemLoadHandler))

	mux.HandleFunc("/write", auth.WithAuth(auth.RoleOperator, tokenRoles, writeFileHandler))
	mux.HandleFunc("/delete", auth.WithAuth(auth.RoleOperator, tokenRoles, deleteFileHandler))
	mux.HandleFunc("/launchapp", auth.WithAuth(auth.RoleOperator, tokenRoles, launchAppHandler))

	mux.HandleFunc("/ydisk/info", auth.WithAuth(auth.RoleViewer, tokenRoles, ydiskInfoHandler))
	mux.HandleFunc("/ydisk/list", auth.WithAuth(auth.RoleViewer, tokenRoles, ydiskListHandler))
	mux.HandleFunc("/ydisk/download", auth.WithAuth(auth.RoleViewer, tokenRoles, ydiskDownloadHandler))
	mux.HandleFunc("/ydisk/search", auth.WithAuth(auth.RoleViewer, tokenRoles, ydiskSearchHandler))

	mux.HandleFunc("/ydisk/upload", auth.WithAuth(auth.RoleOperator, tokenRoles, ydiskUploadHandler))
	mux.HandleFunc("/ydisk/mkdir", auth.WithAuth(auth.RoleOperator, tokenRoles, ydiskCreateDirHandler))
	mux.HandleFunc("/ydisk/delete", auth.WithAuth(auth.RoleOperator, tokenRoles, ydiskDeleteHandler))
	mux.HandleFunc("/ydisk/move", auth.WithAuth(auth.RoleOperator, tokenRoles, ydiskMoveHandler))

	mux.HandleFunc("/browser/open", auth.WithAuth(auth.RoleOperator, tokenRoles, openURLHandler))
	mux.HandleFunc("/browser/fetch", auth.WithAuth(auth.RoleViewer, tokenRoles, fetchURLHandler))
	mux.HandleFunc("/browser/ai-chat", auth.WithAuth(auth.RoleOperator, tokenRoles, sendToAIChatHandler))

	port := os.Getenv("TOOLS_PORT")
	if port == "" {
		port = "8082"
	}

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      requestIDMiddleware(mux),
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
