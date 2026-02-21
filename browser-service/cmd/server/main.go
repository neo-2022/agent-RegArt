// Главный файл browser-service — MCP-микросервис для взаимодействия с браузером.
//
// Этот сервис предоставляет HTTP API для всех операций с браузером:
// - Навигация и получение контента (headless Chrome)
// - Скриншоты и PDF
// - Клавиатура, мышь, вкладки, окна (xdotool/wmctrl)
// - Поиск в интернете (DuckDuckGo, SearXNG)
// - Маскировка под поисковых роботов (Googlebot, YandexBot, Bingbot)
// - Проверка доступности URL (санкции, блокировки, CAPTCHA)
// - Буфер обмена (xclip)
//
// Порт по умолчанию: 8084
// Запуск: go run ./cmd/server/
// Или: go build -o browser-service ./cmd/server/ && ./browser-service
//
// Все ответы — JSON, все сообщения об ошибках — на русском языке.
// Сервис работает независимо от других микросервисов системы.
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/neo-2022/openclaw-memory/browser-service/internal/access"
	"github.com/neo-2022/openclaw-memory/browser-service/internal/browser"
	"github.com/neo-2022/openclaw-memory/browser-service/internal/crawler"
	"github.com/neo-2022/openclaw-memory/browser-service/internal/input"
	"github.com/neo-2022/openclaw-memory/browser-service/internal/search"
)

// ============================================================================
// Структуры запросов
// ============================================================================

// URLRequest — запрос с URL (для навигации, скриншотов, PDF, проверки доступности).
type URLRequest struct {
	URL        string `json:"url"`                   // URL для обработки
	OutputPath string `json:"output_path,omitempty"`  // Путь сохранения файла (скриншот, PDF)
	WindowSize string `json:"window_size,omitempty"`  // Размер окна "ширина,высота"
	Visible    bool   `json:"visible,omitempty"`      // Открыть в видимом браузере
}

// JSRequest — запрос на выполнение JavaScript.
type JSRequest struct {
	URL    string `json:"url"`     // URL страницы (контекст)
	JSCode string `json:"js_code"` // JavaScript-код
}

// KeyRequest — запрос на нажатие клавиш.
type KeyRequest struct {
	Keys     string `json:"keys"`                 // Клавиша или комбинация
	WindowID int    `json:"window_id,omitempty"`  // ID окна (0 = текущее)
}

// TypeRequest — запрос на ввод текста.
type TypeRequest struct {
	Text     string `json:"text"`                 // Текст для ввода
	WindowID int    `json:"window_id,omitempty"`  // ID окна
	Delay    int    `json:"delay,omitempty"`       // Задержка между символами (мс)
}

// MouseClickRequest — запрос на клик мышью.
type MouseClickRequest struct {
	X       int `json:"x"`                  // Координата X
	Y       int `json:"y"`                  // Координата Y
	Button  int `json:"button,omitempty"`   // Кнопка (1=левая, 2=средняя, 3=правая)
	Clicks  int `json:"clicks,omitempty"`   // Количество кликов
}

// MouseMoveRequest — запрос на перемещение мыши.
type MouseMoveRequest struct {
	X int `json:"x"` // Координата X
	Y int `json:"y"` // Координата Y
}

// MouseScrollRequest — запрос на прокрутку.
type MouseScrollRequest struct {
	Direction string `json:"direction"` // "up" или "down"
	Clicks    int    `json:"clicks"`    // Количество шагов
}

// MouseDragRequest — запрос на drag&drop.
type MouseDragRequest struct {
	FromX int `json:"from_x"` // Начальная X
	FromY int `json:"from_y"` // Начальная Y
	ToX   int `json:"to_x"`   // Конечная X
	ToY   int `json:"to_y"`   // Конечная Y
}

// TabRequest — запрос на действие с вкладкой.
type TabRequest struct {
	Action string `json:"action"` // new, close, next, prev, reopen, goto, reload, etc.
	Param  string `json:"param,omitempty"` // Дополнительный параметр
}

// WindowRequest — запрос на действие с окном.
type WindowRequest struct {
	Action string `json:"action"` // list, activate, close, minimize, maximize, etc.
	Target string `json:"target,omitempty"` // ID окна
	Params string `json:"params,omitempty"` // Доп. параметры (x,y,w,h)
}

// ClipboardRequest — запрос на действие с буфером обмена.
type ClipboardRequest struct {
	Action string `json:"action"` // copy, paste, clear
	Text   string `json:"text,omitempty"` // Текст для копирования
}

// ZoomRequest — запрос на масштабирование.
type ZoomRequest struct {
	Action string `json:"action"` // in, out, reset
}

// FindRequest — запрос на поиск текста на странице.
type FindRequest struct {
	Text string `json:"text"` // Текст для поиска
}

// SearchRequest — запрос на интернет-поиск.
type SearchRequest struct {
	Query          string `json:"query"`                      // Поисковый запрос
	MaxResults     int    `json:"max_results,omitempty"`      // Макс. кол-во результатов
	Engine         string `json:"engine,omitempty"`           // Предпочитаемый поисковик
	CustomInstance string `json:"custom_instance,omitempty"`  // URL своего SearXNG
}

// CrawlRequest — запрос на краулинг с маскировкой.
type CrawlRequest struct {
	URL  string `json:"url"`            // URL для загрузки
	Mode string `json:"mode,omitempty"` // Режим: googlebot, yandexbot, bingbot, mailru, normal, auto
}

// CheckURLsRequest — запрос на проверку нескольких URL.
type CheckURLsRequest struct {
	URLs []string `json:"urls"` // Список URL
}

// ============================================================================
// Обработчики HTTP-запросов
// ============================================================================

// --- Навигация и контент ---

// handleGetDOM — получает DOM-контент страницы через headless Chrome.
// POST /browser/dom
func handleGetDOM(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpError(w, "Метод не поддерживается. Используйте POST.", http.StatusMethodNotAllowed)
		return
	}
	var req URLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, "Некорректный JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	result := browser.GetDOM(req.URL)
	jsonResponse(w, result)
}

// handleOpenVisible — открывает URL в видимом браузере (для "покажи мне").
// POST /browser/open
func handleOpenVisible(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpError(w, "Метод не поддерживается", http.StatusMethodNotAllowed)
		return
	}
	var req URLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, "Некорректный JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	result := browser.OpenVisible(req.URL)
	jsonResponse(w, result)
}

// handleScreenshot — делает скриншот страницы.
// POST /browser/screenshot
func handleScreenshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpError(w, "Метод не поддерживается", http.StatusMethodNotAllowed)
		return
	}
	var req URLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, "Некорректный JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	result := browser.Screenshot(req.URL, req.OutputPath, req.WindowSize)
	jsonResponse(w, result)
}

// handlePrintToPDF — конвертирует страницу в PDF.
// POST /browser/pdf
func handlePrintToPDF(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpError(w, "Метод не поддерживается", http.StatusMethodNotAllowed)
		return
	}
	var req URLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, "Некорректный JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	result := browser.PrintToPDF(req.URL, req.OutputPath)
	jsonResponse(w, result)
}

// handleGetText — получает текстовое содержимое страницы (без HTML).
// POST /browser/text
func handleGetText(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpError(w, "Метод не поддерживается", http.StatusMethodNotAllowed)
		return
	}
	var req URLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, "Некорректный JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	result := browser.GetText(req.URL)
	jsonResponse(w, result)
}

// handleGetTitle — получает заголовок страницы.
// POST /browser/title
func handleGetTitle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpError(w, "Метод не поддерживается", http.StatusMethodNotAllowed)
		return
	}
	var req URLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, "Некорректный JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	result := browser.GetTitle(req.URL)
	jsonResponse(w, result)
}

// handleExecuteJS — выполняет JavaScript на странице.
// POST /browser/js
func handleExecuteJS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpError(w, "Метод не поддерживается", http.StatusMethodNotAllowed)
		return
	}
	var req JSRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, "Некорректный JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	result := browser.ExecuteJS(req.URL, req.JSCode)
	jsonResponse(w, result)
}

// handleDetectCaptcha — проверяет страницу на CAPTCHA.
// POST /browser/captcha
func handleDetectCaptcha(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpError(w, "Метод не поддерживается", http.StatusMethodNotAllowed)
		return
	}
	var req URLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, "Некорректный JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	result := browser.DetectCaptcha(req.URL)
	jsonResponse(w, result)
}

// --- Ввод и управление ---

// handleKeyPress — нажимает клавишу или комбинацию.
// POST /input/key
func handleKeyPress(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpError(w, "Метод не поддерживается", http.StatusMethodNotAllowed)
		return
	}
	var req KeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, "Некорректный JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	result := input.KeyPress(req.Keys, req.WindowID)
	jsonResponse(w, result)
}

// handleTypeText — вводит текст посимвольно.
// POST /input/type
func handleTypeText(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpError(w, "Метод не поддерживается", http.StatusMethodNotAllowed)
		return
	}
	var req TypeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, "Некорректный JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	result := input.TypeText(req.Text, req.WindowID, req.Delay)
	jsonResponse(w, result)
}

// handleMouseClick — кликает мышью.
// POST /input/click
func handleMouseClick(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpError(w, "Метод не поддерживается", http.StatusMethodNotAllowed)
		return
	}
	var req MouseClickRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, "Некорректный JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Button == 0 {
		req.Button = 1
	}
	if req.Clicks == 0 {
		req.Clicks = 1
	}
	result := input.MouseClick(req.X, req.Y, req.Button, req.Clicks)
	jsonResponse(w, result)
}

// handleMouseMove — перемещает курсор мыши.
// POST /input/move
func handleMouseMove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpError(w, "Метод не поддерживается", http.StatusMethodNotAllowed)
		return
	}
	var req MouseMoveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, "Некорректный JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	result := input.MouseMove(req.X, req.Y)
	jsonResponse(w, result)
}

// handleMouseScroll — прокручивает колесо мыши.
// POST /input/scroll
func handleMouseScroll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpError(w, "Метод не поддерживается", http.StatusMethodNotAllowed)
		return
	}
	var req MouseScrollRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, "Некорректный JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Clicks == 0 {
		req.Clicks = 3
	}
	result := input.MouseScroll(req.Direction, req.Clicks)
	jsonResponse(w, result)
}

// handleMouseDrag — перетаскивает элемент (drag&drop).
// POST /input/drag
func handleMouseDrag(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpError(w, "Метод не поддерживается", http.StatusMethodNotAllowed)
		return
	}
	var req MouseDragRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, "Некорректный JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	result := input.MouseDrag(req.FromX, req.FromY, req.ToX, req.ToY)
	jsonResponse(w, result)
}

// handleTabAction — управление вкладками браузера.
// POST /input/tab
func handleTabAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpError(w, "Метод не поддерживается", http.StatusMethodNotAllowed)
		return
	}
	var req TabRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, "Некорректный JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	result := input.TabAction(req.Action, req.Param)
	jsonResponse(w, result)
}

// handleWindowAction — управление окнами.
// POST /input/window
func handleWindowAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpError(w, "Метод не поддерживается", http.StatusMethodNotAllowed)
		return
	}
	var req WindowRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, "Некорректный JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	result := input.WindowAction(req.Action, req.Target, req.Params)
	jsonResponse(w, result)
}

// handleClipboard — операции с буфером обмена.
// POST /input/clipboard
func handleClipboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpError(w, "Метод не поддерживается", http.StatusMethodNotAllowed)
		return
	}
	var req ClipboardRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, "Некорректный JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	result := input.ClipboardAction(req.Action, req.Text)
	jsonResponse(w, result)
}

// handleZoom — масштабирование страницы.
// POST /input/zoom
func handleZoom(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpError(w, "Метод не поддерживается", http.StatusMethodNotAllowed)
		return
	}
	var req ZoomRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, "Некорректный JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	result := input.ZoomAction(req.Action)
	jsonResponse(w, result)
}

// handleDevTools — открытие/закрытие DevTools.
// POST /input/devtools
func handleDevTools(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpError(w, "Метод не поддерживается", http.StatusMethodNotAllowed)
		return
	}
	result := input.ToggleDevTools()
	jsonResponse(w, result)
}

// handleFindText — поиск текста на странице.
// POST /input/find
func handleFindText(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpError(w, "Метод не поддерживается", http.StatusMethodNotAllowed)
		return
	}
	var req FindRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, "Некорректный JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	result := input.FindText(req.Text)
	jsonResponse(w, result)
}

// handleGetActiveWindow — информация об активном окне.
// GET /input/active-window
func handleGetActiveWindow(w http.ResponseWriter, r *http.Request) {
	result := input.GetActiveWindow()
	jsonResponse(w, result)
}

// handleGetMouseLocation — текущие координаты курсора.
// GET /input/mouse-location
func handleGetMouseLocation(w http.ResponseWriter, r *http.Request) {
	result := input.GetMouseLocation()
	jsonResponse(w, result)
}

// handleGetScreenResolution — разрешение экрана.
// GET /input/screen-resolution
func handleGetScreenResolution(w http.ResponseWriter, r *http.Request) {
	result := input.GetScreenResolution()
	jsonResponse(w, result)
}

// --- Поиск в интернете ---

// handleSearch — поиск в интернете.
// POST /search
func handleSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpError(w, "Метод не поддерживается", http.StatusMethodNotAllowed)
		return
	}
	var req SearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, "Некорректный JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	result := search.Search(req.Query, req.MaxResults, req.Engine)
	jsonResponse(w, result)
}

// handleSearchDuckDuckGo — поиск через DuckDuckGo.
// POST /search/duckduckgo
func handleSearchDuckDuckGo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpError(w, "Метод не поддерживается", http.StatusMethodNotAllowed)
		return
	}
	var req SearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, "Некорректный JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	result := search.SearchDuckDuckGo(req.Query, req.MaxResults)
	jsonResponse(w, result)
}

// handleSearchSearXNG — поиск через SearXNG.
// POST /search/searxng
func handleSearchSearXNG(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpError(w, "Метод не поддерживается", http.StatusMethodNotAllowed)
		return
	}
	var req SearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, "Некорректный JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	result := search.SearchSearXNG(req.Query, req.MaxResults, req.CustomInstance)
	jsonResponse(w, result)
}

// --- Краулер (маскировка) ---

// handleCrawl — получить контент с маскировкой под робота.
// POST /crawler/fetch
func handleCrawl(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpError(w, "Метод не поддерживается", http.StatusMethodNotAllowed)
		return
	}
	var req CrawlRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, "Некорректный JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	var result crawler.CrawlResult
	if req.Mode == "auto" || req.Mode == "" {
		result = crawler.FetchWithAutoMode(req.URL)
	} else {
		result = crawler.Fetch(req.URL, crawler.BotMode(req.Mode))
	}
	jsonResponse(w, result)
}

// handleCrawlRobotsTxt — получить robots.txt сайта.
// POST /crawler/robots
func handleCrawlRobotsTxt(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpError(w, "Метод не поддерживается", http.StatusMethodNotAllowed)
		return
	}
	var req CrawlRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, "Некорректный JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	mode := crawler.BotMode(req.Mode)
	if mode == "" {
		mode = crawler.BotGooglebot
	}
	result := crawler.FetchRobotsTxt(req.URL, mode)
	jsonResponse(w, result)
}

// handleCrawlModes — список доступных режимов маскировки.
// GET /crawler/modes
func handleCrawlModes(w http.ResponseWriter, r *http.Request) {
	modes := crawler.GetAvailableModes()
	jsonResponse(w, modes)
}

// --- Проверка доступности ---

// handleCheckURL — проверить доступность URL.
// POST /access/check
func handleCheckURL(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpError(w, "Метод не поддерживается", http.StatusMethodNotAllowed)
		return
	}
	var req URLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, "Некорректный JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	result := access.CheckURL(req.URL)
	jsonResponse(w, result)
}

// handleCheckMultipleURLs — проверить доступность нескольких URL.
// POST /access/check-multiple
func handleCheckMultipleURLs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpError(w, "Метод не поддерживается", http.StatusMethodNotAllowed)
		return
	}
	var req CheckURLsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, "Некорректный JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	results := access.CheckMultipleURLs(req.URLs)
	jsonResponse(w, results)
}

// --- Служебные ---

// handleHealth — проверка здоровья сервиса.
// GET /health
func handleHealth(w http.ResponseWriter, r *http.Request) {
	chromeBin, chromeErr := browser.FindChromeBinary()
	health := map[string]interface{}{
		"status":  "ok",
		"service": "browser-service",
		"port":    getPort(),
	}
	if chromeErr != nil {
		health["chrome"] = "не найден"
		health["chrome_error"] = chromeErr.Error()
	} else {
		health["chrome"] = chromeBin
	}
	jsonResponse(w, health)
}

// handleInfo — информация о сервисе и доступных эндпоинтах.
// GET /info
func handleInfo(w http.ResponseWriter, r *http.Request) {
	info := map[string]interface{}{
		"service":     "browser-service",
		"version":     "1.0.0",
		"description": "MCP-микросервис для взаимодействия с браузером",
		"endpoints": map[string]interface{}{
			"browser": []string{
				"POST /browser/dom — получить DOM страницы",
				"POST /browser/open — открыть URL в видимом браузере",
				"POST /browser/screenshot — скриншот страницы",
				"POST /browser/pdf — сохранить как PDF",
				"POST /browser/text — текст страницы без HTML",
				"POST /browser/title — заголовок страницы",
				"POST /browser/js — выполнить JavaScript",
				"POST /browser/captcha — проверить на CAPTCHA",
			},
			"input": []string{
				"POST /input/key — нажать клавишу",
				"POST /input/type — ввести текст",
				"POST /input/click — клик мышью",
				"POST /input/move — переместить мышь",
				"POST /input/scroll — прокрутка",
				"POST /input/drag — drag & drop",
				"POST /input/tab — управление вкладками",
				"POST /input/window — управление окнами",
				"POST /input/clipboard — буфер обмена",
				"POST /input/zoom — масштабирование",
				"POST /input/devtools — открыть DevTools",
				"POST /input/find — поиск текста",
				"GET /input/active-window — активное окно",
				"GET /input/mouse-location — позиция мыши",
				"GET /input/screen-resolution — разрешение экрана",
			},
			"search": []string{
				"POST /search — универсальный поиск",
				"POST /search/duckduckgo — поиск через DuckDuckGo",
				"POST /search/searxng — поиск через SearXNG",
			},
			"crawler": []string{
				"POST /crawler/fetch — загрузить с маскировкой",
				"POST /crawler/robots — получить robots.txt",
				"GET /crawler/modes — режимы маскировки",
			},
			"access": []string{
				"POST /access/check — проверить доступность URL",
				"POST /access/check-multiple — проверить несколько URL",
			},
			"service": []string{
				"GET /health — здоровье сервиса",
				"GET /info — информация о сервисе",
			},
		},
	}
	jsonResponse(w, info)
}

// ============================================================================
// Вспомогательные функции
// ============================================================================

// jsonResponse — отправляет JSON-ответ клиенту.
func jsonResponse(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	json.NewEncoder(w).Encode(data)
}

// httpError — отправляет JSON-ошибку клиенту.
func httpError(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// getPort — получает порт из переменной окружения или возвращает 8084.
func getPort() string {
	port := os.Getenv("BROWSER_SERVICE_PORT")
	if port == "" {
		port = "8084"
	}
	if _, err := strconv.Atoi(port); err != nil {
		port = "8084"
	}
	return port
}

// ============================================================================
// Точка входа
// ============================================================================

func main() {
	port := getPort()

	// --- Браузер (навигация, контент) ---
	http.HandleFunc("/browser/dom", handleGetDOM)
	http.HandleFunc("/browser/open", handleOpenVisible)
	http.HandleFunc("/browser/screenshot", handleScreenshot)
	http.HandleFunc("/browser/pdf", handlePrintToPDF)
	http.HandleFunc("/browser/text", handleGetText)
	http.HandleFunc("/browser/title", handleGetTitle)
	http.HandleFunc("/browser/js", handleExecuteJS)
	http.HandleFunc("/browser/captcha", handleDetectCaptcha)

	// --- Ввод и управление ---
	http.HandleFunc("/input/key", handleKeyPress)
	http.HandleFunc("/input/type", handleTypeText)
	http.HandleFunc("/input/click", handleMouseClick)
	http.HandleFunc("/input/move", handleMouseMove)
	http.HandleFunc("/input/scroll", handleMouseScroll)
	http.HandleFunc("/input/drag", handleMouseDrag)
	http.HandleFunc("/input/tab", handleTabAction)
	http.HandleFunc("/input/window", handleWindowAction)
	http.HandleFunc("/input/clipboard", handleClipboard)
	http.HandleFunc("/input/zoom", handleZoom)
	http.HandleFunc("/input/devtools", handleDevTools)
	http.HandleFunc("/input/find", handleFindText)
	http.HandleFunc("/input/active-window", handleGetActiveWindow)
	http.HandleFunc("/input/mouse-location", handleGetMouseLocation)
	http.HandleFunc("/input/screen-resolution", handleGetScreenResolution)

	// --- Поиск ---
	http.HandleFunc("/search", handleSearch)
	http.HandleFunc("/search/duckduckgo", handleSearchDuckDuckGo)
	http.HandleFunc("/search/searxng", handleSearchSearXNG)

	// --- Краулер ---
	http.HandleFunc("/crawler/fetch", handleCrawl)
	http.HandleFunc("/crawler/robots", handleCrawlRobotsTxt)
	http.HandleFunc("/crawler/modes", handleCrawlModes)

	// --- Доступность ---
	http.HandleFunc("/access/check", handleCheckURL)
	http.HandleFunc("/access/check-multiple", handleCheckMultipleURLs)

	// --- Служебные ---
	http.HandleFunc("/health", handleHealth)
	http.HandleFunc("/info", handleInfo)

	log.Printf("=== browser-service запущен на порту %s ===", port)
	log.Printf("Эндпоинты: /browser/*, /input/*, /search/*, /crawler/*, /access/*")
	log.Printf("Информация: GET /info")

	if err := http.ListenAndServe(fmt.Sprintf(":%s", port), nil); err != nil {
		log.Fatalf("Ошибка запуска сервера: %v", err)
	}
}
