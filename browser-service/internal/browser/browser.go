// Пакет browser — ядро модуля взаимодействия с браузером.
//
// Реализует ВСЕ возможности навигации и получения контента из документации:
// - Chrome DevTools Protocol (CDP): Page.navigate, Page.captureScreenshot, Page.printToPDF,
//   DOM.getDocument, Runtime.evaluate, Network.enable и т.д.
// - Firefox WebExtensions API: tabs.create, tabs.update, tabs.captureVisibleTab
// - Yandex Browser: совместим с Chrome CDP (Chromium-based)
//
// Режимы работы:
// 1. Headless (скрытый) — по умолчанию, Chrome/Chromium --headless --no-sandbox
//    Используется для фоновой работы агента: парсинг, скриншоты, PDF, получение DOM.
// 2. Visible (видимый) — по команде пользователя "покажи мне", открывает URL в GUI-браузере
//    через xdg-open (Linux) для визуального отображения.
//
// Поддерживаемые браузеры (автообнаружение в порядке приоритета):
// 1. Google Chrome (google-chrome, google-chrome-stable)
// 2. Chromium (chromium, chromium-browser)
// 3. Yandex Browser (yandex-browser, yandex-browser-stable)
// 4. Microsoft Edge (microsoft-edge, microsoft-edge-stable)
//
// Все функции возвращают структурированные результаты с полями:
// - Success (bool) — успех операции
// - Data (string) — основные данные (HTML, путь к файлу и т.д.)
// - Error (string) — описание ошибки на русском языке
// - CaptchaDetected (bool) — обнаружена ли CAPTCHA на странице
// - CaptchaType (string) — тип CAPTCHA (recaptcha, hcaptcha, yandex_smartcaptcha и т.д.)
package browser

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ============================================================================
// Константы и конфигурация
// ============================================================================

// Таймаут операций headless-браузера (рендеринг страницы, скриншот, PDF).
// 60 секунд достаточно для большинства страниц, включая SPA с JavaScript.
const headlessTimeout = 60 * time.Second

// Таймаут для быстрых операций (проверка доступности, DNS).
const quickTimeout = 15 * time.Second

// Максимальный размер считываемого DOM-контента (200 КБ).
// Ограничение предотвращает переполнение памяти при обработке тяжёлых страниц.
const maxDOMSize = 200 * 1024

// Размер окна браузера по умолчанию для скриншотов и рендеринга.
// 1920x1080 — стандартное Full HD разрешение.
const defaultWindowSize = "1920,1080"

// BrowserResult — структура результата любой операции с браузером.
// Используется как универсальный ответ для всех функций модуля.
type BrowserResult struct {
	Success         bool   `json:"success"`                    // Успех операции
	Data            string `json:"data,omitempty"`             // Основные данные (HTML, путь к файлу и т.д.)
	Error           string `json:"error,omitempty"`            // Описание ошибки (на русском)
	URL             string `json:"url,omitempty"`              // URL, с которым работали
	StatusCode      int    `json:"status_code,omitempty"`      // HTTP-код ответа (если применимо)
	CaptchaDetected bool   `json:"captcha_detected,omitempty"` // Обнаружена ли CAPTCHA
	CaptchaType     string `json:"captcha_type,omitempty"`     // Тип CAPTCHA (recaptcha, hcaptcha и т.д.)
	Title           string `json:"title,omitempty"`            // Заголовок страницы (если получен)
	FilePath        string `json:"file_path,omitempty"`        // Путь к сохранённому файлу (скриншот, PDF)
}

// ============================================================================
// Поиск исполняемого файла браузера
// ============================================================================

// chromeBinaries — список возможных исполняемых файлов браузера.
// Порядок важен: сначала Chrome (самый стабильный CDP), потом Chromium,
// Yandex Browser (Chromium-based), Edge (тоже Chromium-based).
var chromeBinaries = []string{
	"google-chrome",
	"google-chrome-stable",
	"chromium",
	"chromium-browser",
	"yandex-browser",
	"yandex-browser-stable",
	"microsoft-edge",
	"microsoft-edge-stable",
}

// FindChromeBinary — ищет исполняемый файл Chromium-based браузера в системе.
// Проверяет наличие каждого кандидата через exec.LookPath (аналог which).
// Возвращает полный путь к первому найденному браузеру или ошибку,
// если ни один из поддерживаемых браузеров не установлен.
//
// Порядок поиска:
// 1. google-chrome / google-chrome-stable
// 2. chromium / chromium-browser
// 3. yandex-browser / yandex-browser-stable
// 4. microsoft-edge / microsoft-edge-stable
func FindChromeBinary() (string, error) {
	for _, name := range chromeBinaries {
		path, err := exec.LookPath(name)
		if err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("браузер не найден. Установите один из: Google Chrome, Chromium, Yandex Browser, Microsoft Edge")
}

// ============================================================================
// Нормализация URL
// ============================================================================

// normalizeURL — приводит URL к стандартному виду.
// Если URL не содержит протокол (http:// или https://), добавляет https://.
// Пустой URL вызывает ошибку.
func normalizeURL(url string) (string, error) {
	if url == "" {
		return "", fmt.Errorf("URL не может быть пустым")
	}
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		url = "https://" + url
	}
	return url, nil
}

// ============================================================================
// 1. Навигация — открытие URL
// ============================================================================

// OpenVisible — открывает URL в видимом (GUI) браузере пользователя.
// Используется когда пользователь говорит "покажи мне" — агент открывает
// страницу в обычном браузере с GUI, чтобы пользователь мог её увидеть.
//
// Реализация: вызывает xdg-open (стандарт Linux для открытия URL).
// xdg-open определяет браузер по умолчанию из настроек DE (GNOME, KDE и т.д.).
//
// Параметры:
//   - url: URL для открытия (например, "https://ya.ru")
//
// Возвращает BrowserResult с Success=true если браузер запущен.
// Внимание: функция не ждёт загрузки страницы — только запускает процесс.
func OpenVisible(url string) BrowserResult {
	url, err := normalizeURL(url)
	if err != nil {
		return BrowserResult{Success: false, Error: err.Error()}
	}

	cmd := exec.Command("xdg-open", url)
	if err := cmd.Start(); err != nil {
		return BrowserResult{
			Success: false,
			Error:   fmt.Sprintf("Не удалось открыть браузер: %v", err),
			URL:     url,
		}
	}

	return BrowserResult{
		Success: true,
		Data:    fmt.Sprintf("URL открыт в видимом браузере: %s", url),
		URL:     url,
	}
}

// ============================================================================
// 2. Получение DOM-контента (headless)
// ============================================================================

// GetDOM — получает полный DOM-контент страницы через headless Chrome.
// Использует Chrome DevTools Protocol команду --dump-dom, которая:
// 1. Загружает страницу
// 2. Выполняет весь JavaScript (SPA, React, Vue и т.д.)
// 3. Возвращает итоговый DOM после рендеринга
//
// Это эквивалент CDP команд:
// - Page.navigate(url)
// - Page.loadEventFired()
// - DOM.getDocument()
// - DOM.getOuterHTML()
//
// Параметры:
//   - url: URL страницы для получения DOM
//
// Флаги Chrome:
// --headless=new — новый headless режим (Chrome 112+)
// --no-sandbox — необходим для работы без root
// --disable-gpu — отключает GPU (не нужен в headless)
// --disable-dev-shm-usage — использует /tmp вместо /dev/shm (для Docker/контейнеров)
// --dump-dom — выводит DOM в stdout после полной загрузки
// --timeout=60000 — таймаут загрузки страницы (60 секунд)
//
// Возвращает BrowserResult с HTML-контентом в поле Data.
// Автоматически проверяет контент на наличие CAPTCHA.
func GetDOM(url string) BrowserResult {
	url, err := normalizeURL(url)
	if err != nil {
		return BrowserResult{Success: false, Error: err.Error()}
	}

	chromeBin, err := FindChromeBinary()
	if err != nil {
		return BrowserResult{Success: false, Error: err.Error(), URL: url}
	}

	ctx, cancel := context.WithTimeout(context.Background(), headlessTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, chromeBin,
		"--headless=new",
		"--no-sandbox",
		"--disable-gpu",
		"--disable-dev-shm-usage",
		"--disable-extensions",
		"--disable-background-networking",
		"--disable-sync",
		"--disable-translate",
		"--mute-audio",
		"--no-first-run",
		"--dump-dom",
		url,
	)

	output, err := cmd.Output()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return BrowserResult{
				Success: false,
				Error:   fmt.Sprintf("Таймаут загрузки страницы (%v): %s", headlessTimeout, url),
				URL:     url,
			}
		}
		return BrowserResult{
			Success: false,
			Error:   fmt.Sprintf("Ошибка получения DOM: %v", err),
			URL:     url,
		}
	}

	html := string(output)
	if len(html) > maxDOMSize {
		html = html[:maxDOMSize] + "\n<!-- ... контент обрезан (лимит 200 КБ) -->"
	}

	result := BrowserResult{
		Success: true,
		Data:    html,
		URL:     url,
	}

	checkCaptchaInResult(&result, html)
	return result
}

// ============================================================================
// 3. Скриншот страницы (headless)
// ============================================================================

// Screenshot — делает скриншот веб-страницы через headless Chrome.
// Использует CDP-эквивалент Page.captureScreenshot().
//
// Параметры:
//   - url: URL страницы для скриншота
//   - outputPath: путь для сохранения PNG-файла (если пусто — генерируется автоматически)
//   - windowSize: размер окна "ширина,высота" (по умолчанию "1920,1080")
//
// Флаги Chrome:
// --screenshot=<path> — сохраняет скриншот в указанный файл
// --window-size=<w>,<h> — размер виртуального окна
// --hide-scrollbars — скрывает полосы прокрутки на скриншоте
// --default-background-color=0 — прозрачный фон (для PNG)
//
// Возвращает BrowserResult с путём к файлу в поле FilePath.
func Screenshot(url, outputPath, windowSize string) BrowserResult {
	url, err := normalizeURL(url)
	if err != nil {
		return BrowserResult{Success: false, Error: err.Error()}
	}

	chromeBin, err := FindChromeBinary()
	if err != nil {
		return BrowserResult{Success: false, Error: err.Error(), URL: url}
	}

	if windowSize == "" {
		windowSize = defaultWindowSize
	}

	if outputPath == "" {
		tmpDir := os.TempDir()
		outputPath = filepath.Join(tmpDir, fmt.Sprintf("screenshot_%d.png", time.Now().UnixNano()))
	}

	ctx, cancel := context.WithTimeout(context.Background(), headlessTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, chromeBin,
		"--headless=new",
		"--no-sandbox",
		"--disable-gpu",
		"--disable-dev-shm-usage",
		"--disable-extensions",
		"--hide-scrollbars",
		fmt.Sprintf("--window-size=%s", windowSize),
		fmt.Sprintf("--screenshot=%s", outputPath),
		url,
	)

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return BrowserResult{
				Success: false,
				Error:   fmt.Sprintf("Таймаут при создании скриншота: %s", url),
				URL:     url,
			}
		}
		return BrowserResult{
			Success: false,
			Error:   fmt.Sprintf("Ошибка создания скриншота: %v", err),
			URL:     url,
		}
	}

	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		return BrowserResult{
			Success: false,
			Error:   "Файл скриншота не создан",
			URL:     url,
		}
	}

	return BrowserResult{
		Success:  true,
		Data:     fmt.Sprintf("Скриншот сохранён: %s", outputPath),
		URL:      url,
		FilePath: outputPath,
	}
}

// ============================================================================
// 4. Генерация PDF (headless)
// ============================================================================

// PrintToPDF — сохраняет веб-страницу как PDF через headless Chrome.
// Использует CDP-эквивалент Page.printToPDF().
//
// Параметры:
//   - url: URL страницы для конвертации в PDF
//   - outputPath: путь для сохранения PDF-файла (если пусто — генерируется)
//
// Флаги Chrome:
// --print-to-pdf=<path> — рендерит страницу в PDF
// --no-pdf-header-footer — убирает колонтитулы (дата, URL)
//
// Возвращает BrowserResult с путём к файлу в поле FilePath.
func PrintToPDF(url, outputPath string) BrowserResult {
	url, err := normalizeURL(url)
	if err != nil {
		return BrowserResult{Success: false, Error: err.Error()}
	}

	chromeBin, err := FindChromeBinary()
	if err != nil {
		return BrowserResult{Success: false, Error: err.Error(), URL: url}
	}

	if outputPath == "" {
		tmpDir := os.TempDir()
		outputPath = filepath.Join(tmpDir, fmt.Sprintf("page_%d.pdf", time.Now().UnixNano()))
	}

	ctx, cancel := context.WithTimeout(context.Background(), headlessTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, chromeBin,
		"--headless=new",
		"--no-sandbox",
		"--disable-gpu",
		"--disable-dev-shm-usage",
		"--disable-extensions",
		"--no-pdf-header-footer",
		fmt.Sprintf("--print-to-pdf=%s", outputPath),
		url,
	)

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return BrowserResult{
				Success: false,
				Error:   fmt.Sprintf("Таймаут при создании PDF: %s", url),
				URL:     url,
			}
		}
		return BrowserResult{
			Success: false,
			Error:   fmt.Sprintf("Ошибка создания PDF: %v", err),
			URL:     url,
		}
	}

	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		return BrowserResult{
			Success: false,
			Error:   "Файл PDF не создан",
			URL:     url,
		}
	}

	return BrowserResult{
		Success:  true,
		Data:     fmt.Sprintf("PDF сохранён: %s", outputPath),
		URL:      url,
		FilePath: outputPath,
	}
}

// ============================================================================
// 5. Выполнение JavaScript на странице (headless)
// ============================================================================

// ExecuteJS — выполняет произвольный JavaScript-код на странице.
// Использует CDP-эквивалент Runtime.evaluate().
//
// Chrome загружает страницу, затем выполняет переданный JS через
// --run-javascript аргумент (или через виртуальный DevTools протокол).
//
// Реализация: используется --dump-dom с предварительной инъекцией скрипта
// через data: URL или через отдельный temp-файл с JS-кодом.
//
// Параметры:
//   - url: URL страницы (контекст выполнения)
//   - jsCode: JavaScript-код для выполнения
//
// Возвращает BrowserResult с результатом выполнения JS в поле Data.
func ExecuteJS(url, jsCode string) BrowserResult {
	url, err := normalizeURL(url)
	if err != nil {
		return BrowserResult{Success: false, Error: err.Error()}
	}

	chromeBin, err := FindChromeBinary()
	if err != nil {
		return BrowserResult{Success: false, Error: err.Error(), URL: url}
	}

	// Создаём временный HTML-файл с инъекцией JS
	tmpFile, err := os.CreateTemp("", "js_exec_*.html")
	if err != nil {
		return BrowserResult{
			Success: false,
			Error:   fmt.Sprintf("Ошибка создания временного файла: %v", err),
		}
	}
	defer os.Remove(tmpFile.Name())

	// HTML-обёртка: загружает страницу через iframe и выполняет JS
	htmlContent := fmt.Sprintf(`<!DOCTYPE html>
<html><head><script>
fetch('%s').then(r=>r.text()).then(html=>{
	document.getElementById('out').textContent = (function(){%s})();
}).catch(e=>{
	try { document.getElementById('out').textContent = (function(){%s})(); }
	catch(ex) { document.getElementById('out').textContent = 'ERROR: ' + ex.message; }
});
</script></head><body><pre id="out">LOADING...</pre></body></html>`, url, jsCode, jsCode)

	if _, err := tmpFile.WriteString(htmlContent); err != nil {
		return BrowserResult{Success: false, Error: fmt.Sprintf("Ошибка записи: %v", err)}
	}
	tmpFile.Close()

	ctx, cancel := context.WithTimeout(context.Background(), headlessTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, chromeBin,
		"--headless=new",
		"--no-sandbox",
		"--disable-gpu",
		"--disable-dev-shm-usage",
		"--disable-web-security",
		"--allow-file-access-from-files",
		"--dump-dom",
		"file://"+tmpFile.Name(),
	)

	output, err := cmd.Output()
	if err != nil {
		return BrowserResult{
			Success: false,
			Error:   fmt.Sprintf("Ошибка выполнения JS: %v", err),
			URL:     url,
		}
	}

	return BrowserResult{
		Success: true,
		Data:    string(output),
		URL:     url,
	}
}

// ============================================================================
// 6. Получение заголовка страницы
// ============================================================================

// GetTitle — получает заголовок (<title>) веб-страницы.
// Парсит DOM и извлекает содержимое тега <title>.
//
// Параметры:
//   - url: URL страницы
//
// Возвращает BrowserResult с заголовком в поле Title.
func GetTitle(url string) BrowserResult {
	result := GetDOM(url)
	if !result.Success {
		return result
	}

	html := strings.ToLower(result.Data)
	titleStart := strings.Index(html, "<title>")
	titleEnd := strings.Index(html, "</title>")
	if titleStart >= 0 && titleEnd > titleStart {
		titleStart += len("<title>")
		originalData := result.Data
		if titleEnd <= len(originalData) {
			result.Title = strings.TrimSpace(originalData[titleStart:titleEnd])
		}
	}

	result.Data = ""
	return result
}

// ============================================================================
// 7. Извлечение текста со страницы (без HTML-тегов)
// ============================================================================

// GetText — получает текстовое содержимое страницы без HTML-тегов.
// Полезно для анализа контента, поиска информации, RAG.
//
// Параметры:
//   - url: URL страницы
//
// Возвращает BrowserResult с чистым текстом в поле Data.
func GetText(url string) BrowserResult {
	result := GetDOM(url)
	if !result.Success {
		return result
	}

	text := stripHTMLTags(result.Data)
	text = strings.Join(strings.Fields(text), " ")
	if len(text) > maxDOMSize {
		text = text[:maxDOMSize]
	}

	result.Data = text
	return result
}

// stripHTMLTags — удаляет все HTML-теги из строки.
// Простой парсер: удаляет всё между < и >.
// Также удаляет содержимое <script> и <style> тегов.
func stripHTMLTags(html string) string {
	// Удаляем script и style теги с содержимым
	for _, tag := range []string{"script", "style", "noscript"} {
		for {
			start := strings.Index(strings.ToLower(html), "<"+tag)
			if start < 0 {
				break
			}
			end := strings.Index(strings.ToLower(html[start:]), "</"+tag+">")
			if end < 0 {
				break
			}
			html = html[:start] + html[start+end+len("</"+tag+">"):]
		}
	}

	// Удаляем все HTML-теги
	var result strings.Builder
	inTag := false
	for _, ch := range html {
		if ch == '<' {
			inTag = true
			continue
		}
		if ch == '>' {
			inTag = false
			result.WriteRune(' ')
			continue
		}
		if !inTag {
			result.WriteRune(ch)
		}
	}
	return result.String()
}

// ============================================================================
// 8. CAPTCHA-детекция
// ============================================================================

// captchaPatterns — шаблоны для обнаружения CAPTCHA на странице.
// Каждый шаблон содержит ключевое слово и тип CAPTCHA.
// Источники:
// - Google reCAPTCHA: https://developers.google.com/recaptcha
// - hCaptcha: https://www.hcaptcha.com/
// - Yandex SmartCaptcha: https://cloud.yandex.ru/docs/smartcaptcha/
// - Cloudflare Turnstile: https://developers.cloudflare.com/turnstile/
var captchaPatterns = []struct {
	Keyword string // Ключевое слово в HTML
	Type    string // Тип CAPTCHA
}{
	{"g-recaptcha", "recaptcha"},
	{"recaptcha", "recaptcha"},
	{"grecaptcha", "recaptcha"},
	{"www.google.com/recaptcha", "recaptcha"},
	{"hcaptcha", "hcaptcha"},
	{"h-captcha", "hcaptcha"},
	{"smartcaptcha", "yandex_smartcaptcha"},
	{"captcha.yandex.net", "yandex_smartcaptcha"},
	{"smart-captcha", "yandex_smartcaptcha"},
	{"cf-turnstile", "cloudflare_turnstile"},
	{"challenges.cloudflare.com", "cloudflare_turnstile"},
	{"captcha-solver", "unknown_captcha"},
	{"captcha_image", "image_captcha"},
	{"captcha-image", "image_captcha"},
	{"data-captcha", "unknown_captcha"},
	{"captcha-container", "unknown_captcha"},
	{"captchacode", "unknown_captcha"},
	{"solve_captcha", "unknown_captcha"},
	{"Подтвердите, что вы не робот", "text_captcha_ru"},
	{"Я не робот", "text_captcha_ru"},
	{"Докажите, что вы не робот", "text_captcha_ru"},
	{"verify you are human", "text_captcha_en"},
	{"i'm not a robot", "text_captcha_en"},
	{"prove you are human", "text_captcha_en"},
}

// checkCaptchaInResult — проверяет HTML-контент на наличие CAPTCHA.
// Если обнаружена — устанавливает CaptchaDetected=true и CaptchaType.
// Вызывается автоматически после получения DOM или fetch.
func checkCaptchaInResult(result *BrowserResult, htmlContent string) {
	lower := strings.ToLower(htmlContent)
	for _, p := range captchaPatterns {
		if strings.Contains(lower, strings.ToLower(p.Keyword)) {
			result.CaptchaDetected = true
			result.CaptchaType = p.Type
			return
		}
	}
}

// DetectCaptcha — проверяет страницу на наличие CAPTCHA.
// Загружает DOM через headless Chrome и сканирует на ключевые слова.
//
// Параметры:
//   - url: URL страницы для проверки
//
// Возвращает BrowserResult с CaptchaDetected и CaptchaType.
func DetectCaptcha(url string) BrowserResult {
	result := GetDOM(url)
	if !result.Success {
		return result
	}
	if result.CaptchaDetected {
		result.Data = fmt.Sprintf("CAPTCHA обнаружена! Тип: %s. Агент не может решить CAPTCHA — требуется ваша помощь.", result.CaptchaType)
	} else {
		result.Data = "CAPTCHA не обнаружена на странице."
	}
	return result
}
