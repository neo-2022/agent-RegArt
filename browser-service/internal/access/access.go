// Пакет access — модуль проверки доступности ресурсов и санкционных ограничений.
//
// Реализует комплексную проверку доступности URL с учётом:
// - Сетевой доступности (DNS, TCP-соединение)
// - HTTP-статуса (403, 451, 503 и др.)
// - Блокировок Роскомнадзора
// - Санкционных ограничений (зарубежные ресурсы, заблокированные для РФ)
// - SSL/TLS сертификатов
// - Геоблокировок (country-based blocking)
//
// Все сообщения об ошибках — на русском языке.
// Каждая проверка возвращает подробное описание проблемы и рекомендации.
//
// Используется:
// - Перед каждым запросом краулера для предупреждения пользователя
// - Для диагностики проблем с доступом к ресурсам
// - Для проверки цепочки редиректов
package access

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// ============================================================================
// Константы
// ============================================================================

// Таймаут для проверки доступности (10 секунд).
const checkTimeout = 10 * time.Second

// Таймаут DNS-резолвинга (5 секунд).
const dnsTimeout = 5 * time.Second

// Таймаут TCP-соединения (5 секунд).
const tcpTimeout = 5 * time.Second

// ============================================================================
// Структуры
// ============================================================================

// AccessCheckResult — результат проверки доступности URL.
type AccessCheckResult struct {
	URL             string `json:"url"`                        // Проверяемый URL
	Accessible      bool   `json:"accessible"`                 // Доступен ли ресурс
	StatusCode      int    `json:"status_code,omitempty"`      // HTTP-код ответа
	Error           string `json:"error,omitempty"`            // Описание проблемы (на русском)
	Warning         string `json:"warning,omitempty"`          // Предупреждение (на русском)
	Recommendation  string `json:"recommendation,omitempty"`   // Рекомендации по исправлению
	DNSResolved     bool   `json:"dns_resolved"`               // DNS-имя разрешено
	TCPConnected    bool   `json:"tcp_connected"`              // TCP-соединение установлено
	TLSValid        bool   `json:"tls_valid"`                  // SSL/TLS-сертификат валиден
	Blocked         bool   `json:"blocked"`                    // Ресурс заблокирован
	BlockReason     string `json:"block_reason,omitempty"`     // Причина блокировки
	ResponseTime    int64  `json:"response_time_ms,omitempty"` // Время ответа (мс)
	Server          string `json:"server,omitempty"`           // Заголовок Server
	FinalURL        string `json:"final_url,omitempty"`        // URL после редиректов
	CaptchaDetected bool   `json:"captcha_detected,omitempty"` // Обнаружена CAPTCHA
}

// ============================================================================
// Основная функция проверки
// ============================================================================

// CheckURL — выполняет комплексную проверку доступности URL.
// Последовательно проверяет:
// 1. DNS-резолвинг (разрешение доменного имени)
// 2. TCP-соединение (подключение к серверу)
// 3. TLS/SSL-сертификат (для HTTPS)
// 4. HTTP-ответ (код статуса, заголовки)
// 5. Наличие блокировок и CAPTCHA
//
// Параметры:
//   - url: URL для проверки
//
// Возвращает AccessCheckResult с подробной информацией о доступности.
func CheckURL(rawURL string) AccessCheckResult {
	if rawURL == "" {
		return AccessCheckResult{
			URL:        rawURL,
			Accessible: false,
			Error:      "URL не может быть пустым",
		}
	}

	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		rawURL = "https://" + rawURL
	}

	result := AccessCheckResult{URL: rawURL}
	startTime := time.Now()

	// Извлекаем хост из URL
	host := extractHost(rawURL)
	if host == "" {
		result.Error = "Не удалось извлечь хост из URL"
		return result
	}

	// 1. Проверка DNS
	result.DNSResolved = checkDNS(host)
	if !result.DNSResolved {
		result.Error = fmt.Sprintf("DNS-ошибка: домен '%s' не найден.\n"+
			"Возможные причины:\n"+
			"- Домен не существует или просрочен\n"+
			"- DNS-блокировка интернет-провайдером (Роскомнадзор)\n"+
			"- Проблемы с DNS-сервером", host)
		result.Recommendation = "Попробуйте:\n" +
			"1. Проверить правильность URL\n" +
			"2. Сменить DNS на 8.8.8.8 (Google) или 77.88.8.8 (Яндекс)\n" +
			"3. Использовать VPN"
		return result
	}

	// 2. Проверка TCP-соединения
	port := "443"
	if strings.HasPrefix(rawURL, "http://") {
		port = "80"
	}
	result.TCPConnected = checkTCP(host, port)
	if !result.TCPConnected {
		result.Error = fmt.Sprintf("Не удалось установить TCP-соединение с %s:%s.\n"+
			"Возможные причины:\n"+
			"- Сервер не работает\n"+
			"- Порт заблокирован файрволом\n"+
			"- IP-адрес заблокирован провайдером (DPI/Роскомнадзор)", host, port)
		result.Blocked = true
		result.BlockReason = "TCP-соединение заблокировано"
		result.Recommendation = "Попробуйте использовать VPN или прокси-сервер"
		return result
	}

	// 3. Проверка TLS (для HTTPS)
	if strings.HasPrefix(rawURL, "https://") {
		result.TLSValid = checkTLS(host)
		if !result.TLSValid {
			result.Warning = fmt.Sprintf("SSL/TLS-сертификат для %s невалиден или просрочен.\n"+
				"Это может быть признаком:\n"+
				"- Просроченного сертификата\n"+
				"- MITM-атаки\n"+
				"- Подмены сертификата DPI-оборудованием", host)
		}
	}

	// 4. HTTP-запрос
	httpResult := checkHTTP(rawURL)
	result.StatusCode = httpResult.statusCode
	result.Server = httpResult.server
	result.FinalURL = httpResult.finalURL
	result.CaptchaDetected = httpResult.captcha

	// 5. Анализ статуса
	result.ResponseTime = time.Since(startTime).Milliseconds()

	switch {
	case result.StatusCode >= 200 && result.StatusCode < 400:
		result.Accessible = true
		if result.CaptchaDetected {
			result.Warning = "Страница доступна, но содержит CAPTCHA. Агент не может автоматически решить CAPTCHA."
		}
	case result.StatusCode == 403:
		result.Blocked = true
		result.BlockReason = "Доступ запрещён (HTTP 403)"
		result.Error = fmt.Sprintf("Доступ к %s запрещён (HTTP 403).\n"+
			"Возможные причины:\n"+
			"- Геоблокировка (доступ из РФ запрещён)\n"+
			"- Сайт заблокировал ваш IP-адрес\n"+
			"- Требуется авторизация", rawURL)
		result.Recommendation = "Попробуйте VPN или другой режим маскировки (crawler)"
	case result.StatusCode == 451:
		result.Blocked = true
		result.BlockReason = "Недоступен по юридическим причинам (HTTP 451)"
		result.Error = fmt.Sprintf("Ресурс %s недоступен по юридическим причинам (HTTP 451).\n"+
			"Контент заблокирован по решению суда или из-за санкционных ограничений.", rawURL)
		result.Recommendation = "Использование VPN может помочь, но учтите юридические риски"
	case result.StatusCode == 429:
		result.Error = fmt.Sprintf("Слишком много запросов к %s (HTTP 429). Подождите и попробуйте позже.", rawURL)
		result.Accessible = true
	case result.StatusCode >= 500:
		result.Error = fmt.Sprintf("Ошибка сервера %s (HTTP %d). Сервер временно недоступен.", rawURL, result.StatusCode)
	case result.StatusCode == 0:
		result.Error = fmt.Sprintf("Не удалось получить HTTP-ответ от %s. Ресурс может быть заблокирован.", rawURL)
		result.Blocked = true
	}

	return result
}

// CheckMultipleURLs — проверяет доступность нескольких URL одновременно.
// Полезно для массовой проверки списка ресурсов.
//
// Параметры:
//   - urls: список URL для проверки
//
// Возвращает массив AccessCheckResult для каждого URL.
func CheckMultipleURLs(urls []string) []AccessCheckResult {
	results := make([]AccessCheckResult, len(urls))
	for i, u := range urls {
		results[i] = CheckURL(u)
	}
	return results
}

// ============================================================================
// Вспомогательные функции
// ============================================================================

// extractHost — извлекает хост (домен) из URL.
func extractHost(rawURL string) string {
	url := rawURL
	// Удаляем протокол
	if idx := strings.Index(url, "://"); idx >= 0 {
		url = url[idx+3:]
	}
	// Удаляем путь
	if idx := strings.Index(url, "/"); idx >= 0 {
		url = url[:idx]
	}
	// Удаляем порт
	if idx := strings.LastIndex(url, ":"); idx >= 0 {
		url = url[:idx]
	}
	return url
}

// checkDNS — проверяет DNS-резолвинг домена.
func checkDNS(host string) bool {
	resolver := &net.Resolver{}
	ctx, cancel := contextWithTimeout(dnsTimeout)
	defer cancel()
	_, err := resolver.LookupHost(ctx, host)
	return err == nil
}

// checkTCP — проверяет TCP-соединение с хостом:портом.
func checkTCP(host, port string) bool {
	conn, err := net.DialTimeout("tcp", host+":"+port, tcpTimeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// checkTLS — проверяет валидность TLS-сертификата.
func checkTLS(host string) bool {
	conn, err := tls.DialWithDialer(
		&net.Dialer{Timeout: tcpTimeout},
		"tcp",
		host+":443",
		&tls.Config{InsecureSkipVerify: false},
	)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// httpCheckResult — внутренняя структура для HTTP-проверки.
type httpCheckResult struct {
	statusCode int
	server     string
	finalURL   string
	captcha    bool
}

// checkHTTP — выполняет HTTP HEAD/GET запрос для проверки доступности.
func checkHTTP(rawURL string) httpCheckResult {
	client := &http.Client{
		Timeout: checkTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("слишком много редиректов")
			}
			return nil
		},
	}

	// Сначала пробуем HEAD (быстрее)
	req, err := http.NewRequest("HEAD", rawURL, nil)
	if err != nil {
		return httpCheckResult{}
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return httpCheckResult{}
	}
	defer resp.Body.Close()

	result := httpCheckResult{
		statusCode: resp.StatusCode,
		server:     resp.Header.Get("Server"),
		finalURL:   resp.Request.URL.String(),
	}

	// Для проверки CAPTCHA нужен GET
	if resp.StatusCode == 200 || resp.StatusCode == 403 {
		getReq, err := http.NewRequest("GET", rawURL, nil)
		if err == nil {
			getReq.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
			getResp, err := client.Do(getReq)
			if err == nil {
				defer getResp.Body.Close()
				body, _ := io.ReadAll(io.LimitReader(getResp.Body, 100*1024))
				lower := strings.ToLower(string(body))
				captchaKeywords := []string{
					"captcha", "recaptcha", "hcaptcha", "smartcaptcha",
					"cf-turnstile", "не робот", "not a robot",
				}
				for _, kw := range captchaKeywords {
					if strings.Contains(lower, kw) {
						result.captcha = true
						break
					}
				}
			}
		}
	}

	return result
}

// contextWithTimeout — создаёт стандартный context.Context с таймаутом для DNS-резолвинга.
// Использует стандартный пакет context для совместимости с net.Resolver.
func contextWithTimeout(timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), timeout)
}
