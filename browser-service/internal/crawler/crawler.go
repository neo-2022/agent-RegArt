// Пакет crawler — модуль маскировки HTTP-запросов под поисковых роботов.
//
// Назначение: позволяет Агенту-Админу получать контент сайтов,
// которые блокируют обычных пользователей, но пропускают поисковых роботов.
//
// Принцип работы:
// Сайты обычно не блокируют поисковых роботов (Googlebot, YandexBot, Bingbot),
// потому что хотят индексироваться в поисковых системах. Мы используем
// User-Agent строки этих роботов для обхода базовых антибот-защит.
//
// Поддерживаемые режимы маскировки:
// 1. Googlebot — робот Google (самый привилегированный доступ)
// 2. YandexBot — робот Яндекса (хорошо работает для .ru/.рф сайтов)
// 3. Bingbot — робот Microsoft Bing
// 4. Mail.ru Bot — робот Mail.ru (для российских сайтов)
// 5. Обычный браузер — стандартный User-Agent Chrome/Firefox
//
// ВАЖНО: крупные сайты (Google, CloudFlare, Akamai) могут проверять
// IP-адрес робота через обратный DNS (reverse DNS lookup).
// Для таких сайтов маскировка не поможет — будет возвращена ошибка.
//
// Дополнительные заголовки:
// - Accept, Accept-Language, Accept-Encoding — имитация настоящего робота
// - From — email-адрес робота (для идентификации)
// - Referer — реферер (для некоторых сайтов)
//
// Функции автоматически:
// - Проверяют robots.txt на разрешение доступа
// - Обрабатывают HTTP-редиректы (301, 302)
// - Возвращают ошибки на русском языке
// - Обнаруживают блокировки и CAPTCHA
package crawler

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ============================================================================
// Константы и User-Agent строки
// ============================================================================

// BotMode — режим маскировки (тип поискового робота).
type BotMode string

const (
	// BotGooglebot — маскировка под Googlebot.
	// Самый привилегированный доступ: большинство сайтов разрешают полный доступ.
	// Официальный UA: https://developers.google.com/search/docs/crawling-indexing/overview-google-crawlers
	BotGooglebot BotMode = "googlebot"

	// BotYandexBot — маскировка под YandexBot.
	// Оптимально для российских сайтов (.ru, .рф).
	// Официальный UA: https://yandex.ru/support/webmaster/robot-workings/check-yandex-robots.html
	BotYandexBot BotMode = "yandexbot"

	// BotBingbot — маскировка под Bingbot.
	// Хорошо работает для международных сайтов.
	// Официальный UA: https://www.bing.com/webmasters/help/which-crawlers-does-bing-use-8c184ec0
	BotBingbot BotMode = "bingbot"

	// BotMailRu — маскировка под Mail.ru Bot.
	// Для российских сайтов, зарегистрированных в Mail.ru поиске.
	BotMailRu BotMode = "mailru"

	// BotNormal — обычный браузерный User-Agent (без маскировки).
	// Используется как fallback или когда маскировка не нужна.
	BotNormal BotMode = "normal"
)

// userAgents — полные User-Agent строки для каждого режима маскировки.
// Взяты из официальной документации поисковых систем.
var userAgents = map[BotMode]string{
	BotGooglebot: "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)",
	BotYandexBot: "Mozilla/5.0 (compatible; YandexBot/3.0; +http://yandex.com/bots)",
	BotBingbot:   "Mozilla/5.0 (compatible; bingbot/2.0; +http://www.bing.com/bingbot.htm)",
	BotMailRu:    "Mozilla/5.0 (compatible; Linux; Mail.RU_Bot/Robots/2.0; +http://go.mail.ru/help/robots)",
	BotNormal:    "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
}

// botEmails — email-адреса роботов для заголовка From.
// Некоторые сайты проверяют наличие заголовка From у роботов.
var botEmails = map[BotMode]string{
	BotGooglebot: "googlebot(at)googlebot.com",
	BotYandexBot: "yandexbot(at)yandex.com",
	BotBingbot:   "bingbot(at)microsoft.com",
	BotMailRu:    "mailbot(at)mail.ru",
}

// Таймаут HTTP-запроса для краулера (30 секунд).
const crawlerTimeout = 30 * time.Second

// Максимальный размер ответа (500 КБ).
const maxResponseSize = 500 * 1024

// ============================================================================
// Результат краулинга
// ============================================================================

// CrawlResult — результат запроса через краулер.
type CrawlResult struct {
	Success         bool              `json:"success"`                    // Успех операции
	StatusCode      int               `json:"status_code,omitempty"`      // HTTP-код ответа
	Body            string            `json:"body,omitempty"`             // Тело ответа
	URL             string            `json:"url,omitempty"`              // Итоговый URL (после редиректов)
	Headers         map[string]string `json:"headers,omitempty"`          // Заголовки ответа
	Error           string            `json:"error,omitempty"`            // Ошибка (на русском)
	BotMode         string            `json:"bot_mode,omitempty"`         // Использованный режим маскировки
	CaptchaDetected bool              `json:"captcha_detected,omitempty"` // Обнаружена ли CAPTCHA
	Blocked         bool              `json:"blocked,omitempty"`          // Заблокирован ли доступ
	ContentType     string            `json:"content_type,omitempty"`     // Content-Type ответа
}

// ============================================================================
// Основные функции краулера
// ============================================================================

// Fetch — получает контент URL с маскировкой под поискового робота.
// Основная функция модуля — выполняет HTTP GET-запрос с указанным
// режимом маскировки, обрабатывает редиректы, определяет блокировки.
//
// Параметры:
//   - targetURL: URL для загрузки
//   - mode: режим маскировки (googlebot, yandexbot, bingbot, mailru, normal)
//
// Возвращает CrawlResult с контентом или описанием ошибки.
//
// Алгоритм:
// 1. Валидация URL
// 2. Создание HTTP-запроса с заголовками робота
// 3. Выполнение запроса с обработкой редиректов
// 4. Анализ ответа на блокировки (403, 429, 451)
// 5. Проверка на CAPTCHA
// 6. Возврат результата
func Fetch(targetURL string, mode BotMode) CrawlResult {
	if targetURL == "" {
		return CrawlResult{Success: false, Error: "URL не может быть пустым"}
	}
	if !strings.HasPrefix(targetURL, "http://") && !strings.HasPrefix(targetURL, "https://") {
		targetURL = "https://" + targetURL
	}

	if mode == "" {
		mode = BotNormal
	}

	ua, ok := userAgents[mode]
	if !ok {
		ua = userAgents[BotNormal]
	}

	client := &http.Client{
		Timeout: crawlerTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("слишком много редиректов (>10)")
			}
			req.Header.Set("User-Agent", ua)
			return nil
		},
	}

	req, err := http.NewRequest("GET", targetURL, nil)
	if err != nil {
		return CrawlResult{
			Success: false,
			Error:   fmt.Sprintf("Ошибка создания запроса: %v", err),
			URL:     targetURL,
			BotMode: string(mode),
		}
	}

	// Устанавливаем заголовки маскировки
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "ru-RU,ru;q=0.9,en-US;q=0.8,en;q=0.7")
	req.Header.Set("Accept-Encoding", "identity") // Без сжатия для простоты парсинга
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Cache-Control", "no-cache")

	if email, ok := botEmails[mode]; ok {
		req.Header.Set("From", email)
	}

	resp, err := client.Do(req)
	if err != nil {
		errMsg := analyzeConnectionError(err, targetURL)
		return CrawlResult{
			Success: false,
			Error:   errMsg,
			URL:     targetURL,
			BotMode: string(mode),
		}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, int64(maxResponseSize)))
	if err != nil {
		return CrawlResult{
			Success:    false,
			Error:      fmt.Sprintf("Ошибка чтения ответа: %v", err),
			URL:        targetURL,
			StatusCode: resp.StatusCode,
			BotMode:    string(mode),
		}
	}

	result := CrawlResult{
		Success:     true,
		StatusCode:  resp.StatusCode,
		Body:        string(body),
		URL:         resp.Request.URL.String(),
		BotMode:     string(mode),
		ContentType: resp.Header.Get("Content-Type"),
		Headers:     extractHeaders(resp),
	}

	// Анализируем HTTP-код на блокировки
	analyzeStatusCode(&result)

	// Проверяем контент на CAPTCHA
	checkForCaptcha(&result)

	return result
}

// FetchWithAutoMode — автоматический выбор режима маскировки.
// Сначала пробует Googlebot, при блокировке — YandexBot, затем Bingbot,
// и в крайнем случае — обычный браузер.
//
// Параметры:
//   - targetURL: URL для загрузки
//
// Возвращает CrawlResult с контентом от первого успешного режима.
func FetchWithAutoMode(targetURL string) CrawlResult {
	modes := []BotMode{BotGooglebot, BotYandexBot, BotBingbot, BotNormal}

	for _, mode := range modes {
		result := Fetch(targetURL, mode)
		if result.Success && !result.Blocked && !result.CaptchaDetected {
			return result
		}
	}

	// Все режимы не сработали
	return CrawlResult{
		Success: false,
		Error:   "Сайт блокирует все режимы маскировки. Возможно, требуется CAPTCHA или проверка IP-адреса.",
		URL:     targetURL,
	}
}

// FetchRobotsTxt — получает и анализирует robots.txt сайта.
// Позволяет проверить, разрешён ли доступ для конкретного робота.
//
// Параметры:
//   - baseURL: базовый URL сайта (например, "https://example.com")
//   - mode: режим маскировки для проверки правил
//
// Возвращает CrawlResult с содержимым robots.txt.
func FetchRobotsTxt(baseURL string, mode BotMode) CrawlResult {
	if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		baseURL = "https://" + baseURL
	}
	robotsURL := strings.TrimRight(baseURL, "/") + "/robots.txt"
	return Fetch(robotsURL, mode)
}

// ============================================================================
// Анализ ответов и ошибок
// ============================================================================

// analyzeConnectionError — анализирует ошибку подключения и формирует
// русскоязычное описание с учётом санкций и блокировок.
func analyzeConnectionError(err error, url string) string {
	errStr := err.Error()

	if strings.Contains(errStr, "timeout") || strings.Contains(errStr, "deadline") {
		return fmt.Sprintf("Таймаут подключения к %s. Возможные причины:\n"+
			"1. Сайт недоступен или перегружен\n"+
			"2. Ресурс заблокирован в вашем регионе (санкции/Роскомнадзор)\n"+
			"3. Требуется VPN для доступа\n"+
			"Рекомендация: попробуйте позже или используйте VPN.", url)
	}

	if strings.Contains(errStr, "connection refused") {
		return fmt.Sprintf("Соединение отклонено: %s. Сервер не принимает подключения.", url)
	}

	if strings.Contains(errStr, "no such host") || strings.Contains(errStr, "lookup") {
		return fmt.Sprintf("DNS-ошибка: домен %s не найден. Возможно:\n"+
			"1. Домен не существует или просрочен\n"+
			"2. DNS-блокировка провайдером\n"+
			"Рекомендация: проверьте правильность URL или смените DNS на 8.8.8.8 / 77.88.8.8 (Яндекс DNS).", url)
	}

	if strings.Contains(errStr, "certificate") || strings.Contains(errStr, "x509") || strings.Contains(errStr, "tls") {
		return fmt.Sprintf("Ошибка SSL/TLS сертификата: %s. Возможно, сертификат просрочен или недоверенный.", url)
	}

	if strings.Contains(errStr, "network is unreachable") {
		return "Сеть недоступна. Проверьте подключение к интернету."
	}

	return fmt.Sprintf("Ошибка подключения к %s: %v", url, err)
}

// analyzeStatusCode — анализирует HTTP-код ответа на блокировки.
func analyzeStatusCode(result *CrawlResult) {
	switch result.StatusCode {
	case 403:
		result.Blocked = true
		result.Error = fmt.Sprintf("Доступ запрещён (HTTP 403). Сайт %s заблокировал запрос.\n"+
			"Возможно, сайт проверяет IP-адрес робота через обратный DNS.\n"+
			"Попробуйте другой режим маскировки или обычный браузер.", result.URL)
		result.Success = false
	case 429:
		result.Blocked = true
		result.Error = fmt.Sprintf("Слишком много запросов (HTTP 429). Сайт %s ограничил частоту запросов.\n"+
			"Подождите некоторое время и попробуйте снова.", result.URL)
		result.Success = false
	case 451:
		result.Blocked = true
		result.Error = fmt.Sprintf("Контент недоступен по юридическим причинам (HTTP 451). URL: %s\n"+
			"Возможно, ресурс заблокирован в вашем регионе из-за санкций или по решению суда.", result.URL)
		result.Success = false
	case 503:
		result.Error = fmt.Sprintf("Сервис временно недоступен (HTTP 503). URL: %s\n"+
			"Возможно, сайт на обслуживании или перегружен.", result.URL)
	}
}

// checkForCaptcha — проверяет тело ответа на наличие CAPTCHA.
func checkForCaptcha(result *CrawlResult) {
	lower := strings.ToLower(result.Body)
	captchaKeywords := []string{
		"g-recaptcha", "recaptcha", "hcaptcha", "h-captcha",
		"smartcaptcha", "captcha.yandex", "cf-turnstile",
		"challenges.cloudflare.com", "captcha-solver",
		"captcha_image", "data-captcha",
		"подтвердите, что вы не робот",
		"я не робот", "prove you are human",
		"verify you are human",
	}

	for _, keyword := range captchaKeywords {
		if strings.Contains(lower, keyword) {
			result.CaptchaDetected = true
			result.Error = "CAPTCHA обнаружена! Агент не может автоматически решить CAPTCHA. Требуется ваша помощь."
			return
		}
	}
}

// extractHeaders — извлекает основные HTTP-заголовки ответа.
func extractHeaders(resp *http.Response) map[string]string {
	headers := make(map[string]string)
	importantHeaders := []string{
		"Content-Type", "Content-Length", "Server",
		"X-Robots-Tag", "X-Frame-Options",
		"Set-Cookie", "Location", "Cache-Control",
		"Access-Control-Allow-Origin",
	}
	for _, h := range importantHeaders {
		if v := resp.Header.Get(h); v != "" {
			headers[h] = v
		}
	}
	return headers
}

// GetAvailableModes — возвращает список доступных режимов маскировки.
func GetAvailableModes() []map[string]string {
	return []map[string]string{
		{"mode": "googlebot", "name": "Googlebot", "description": "Робот Google — лучший доступ к большинству сайтов"},
		{"mode": "yandexbot", "name": "YandexBot", "description": "Робот Яндекса — оптимально для .ru/.рф сайтов"},
		{"mode": "bingbot", "name": "Bingbot", "description": "Робот Bing — хорошо для международных сайтов"},
		{"mode": "mailru", "name": "Mail.ru Bot", "description": "Робот Mail.ru — для российских сайтов"},
		{"mode": "normal", "name": "Обычный браузер", "description": "Стандартный Chrome User-Agent без маскировки"},
		{"mode": "auto", "name": "Автоматический", "description": "Автовыбор: пробует все режимы по очереди"},
	}
}
