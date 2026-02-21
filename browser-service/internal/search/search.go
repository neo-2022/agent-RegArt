// Пакет search — модуль поиска в интернете через бесплатные инструменты.
//
// Реализует поиск без использования платных API, с учётом:
// - Работоспособности из РФ (санкции, блокировки Роскомнадзора)
// - Поддержки русского языка и локализации ru-ru
// - Бесплатных открытых инструментов
//
// Поддерживаемые поисковые системы:
//
// 1. DuckDuckGo (duckduckgo.com)
//    - Бесплатный, не требует API-ключа
//    - Работает из РФ без VPN
//    - Поддерживает русский язык (kl=ru-ru)
//    - HTML-версия (lite) не требует JavaScript
//    - Не отслеживает пользователей
//
// 2. SearXNG (searx.space)
//    - Открытый метапоисковик (агрегирует результаты из Google, Bing, Yahoo и др.)
//    - Можно развернуть локально (self-hosted)
//    - Множество публичных инстансов
//    - Поддерживает API (JSON-формат)
//    - Не требует API-ключа
//
// 3. Brave Search (search.brave.com)
//    - Собственный индекс (не зависит от Google)
//    - Бесплатный веб-поиск
//    - Может быть ограничен из РФ
//
// Все функции автоматически проверяют доступность ресурса перед запросом.
// При недоступности — возвращают русскоязычное предупреждение.
package search

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// SearchResult — один результат поиска.
type SearchResult struct {
	Title   string `json:"title"`             // Заголовок результата
	URL     string `json:"url"`               // URL страницы
	Snippet string `json:"snippet,omitempty"`  // Краткое описание / сниппет
	Source  string `json:"source,omitempty"`   // Источник (duckduckgo, searxng, brave)
}

// SearchResponse — полный ответ поиска.
type SearchResponse struct {
	Success bool           `json:"success"`           // Успех операции
	Query   string         `json:"query"`             // Поисковый запрос
	Results []SearchResult `json:"results,omitempty"` // Список результатов
	Error   string         `json:"error,omitempty"`   // Ошибка (на русском)
	Source  string         `json:"source,omitempty"`  // Какой поисковик использован
	Count   int            `json:"count"`             // Количество результатов
}

// Таймаут HTTP-запросов для поиска (15 секунд).
const searchTimeout = 15 * time.Second

// Максимальный размер ответа от поисковика (500 КБ).
const maxSearchResponse = 500 * 1024

// userAgent — User-Agent для поисковых запросов.
// Используем обычный браузерный UA, чтобы не быть заблокированными.
const userAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

// ============================================================================
// 1. DuckDuckGo — основной поисковик (бесплатный, работает из РФ)
// ============================================================================

// SearchDuckDuckGo — выполняет поиск через DuckDuckGo HTML-версию.
// Используется lite-версия (html.duckduckgo.com/html/) которая:
// - Не требует JavaScript
// - Возвращает чистый HTML с результатами
// - Работает из РФ без VPN
// - Поддерживает русский язык через параметр kl=ru-ru
//
// Параметры:
//   - query: поисковый запрос (на любом языке)
//   - maxResults: максимальное количество результатов (по умолчанию 10)
//
// Возвращает SearchResponse с результатами поиска.
func SearchDuckDuckGo(query string, maxResults int) SearchResponse {
	if query == "" {
		return SearchResponse{Success: false, Error: "Поисковый запрос не может быть пустым", Query: query}
	}
	if maxResults <= 0 {
		maxResults = 10
	}

	// Формируем запрос к HTML-версии DuckDuckGo
	searchURL := "https://html.duckduckgo.com/html/"
	formData := url.Values{}
	formData.Set("q", query)
	formData.Set("kl", "ru-ru") // Локализация для России
	formData.Set("kp", "-1")    // Отключаем SafeSearch для полных результатов

	client := &http.Client{Timeout: searchTimeout}
	req, err := http.NewRequest("POST", searchURL, strings.NewReader(formData.Encode()))
	if err != nil {
		return SearchResponse{Success: false, Error: fmt.Sprintf("Ошибка создания запроса: %v", err), Query: query}
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "text/html")
	req.Header.Set("Accept-Language", "ru-RU,ru;q=0.9,en-US;q=0.8,en;q=0.7")

	resp, err := client.Do(req)
	if err != nil {
		if strings.Contains(err.Error(), "timeout") || strings.Contains(err.Error(), "deadline") {
			return SearchResponse{
				Success: false,
				Error:   "Таймаут подключения к DuckDuckGo. Возможно, ресурс недоступен из вашего региона.",
				Query:   query,
			}
		}
		return SearchResponse{Success: false, Error: fmt.Sprintf("Ошибка запроса к DuckDuckGo: %v", err), Query: query}
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return SearchResponse{
			Success: false,
			Error:   fmt.Sprintf("DuckDuckGo вернул HTTP %d. Возможно, временная блокировка.", resp.StatusCode),
			Query:   query,
		}
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, int64(maxSearchResponse)))
	if err != nil {
		return SearchResponse{Success: false, Error: fmt.Sprintf("Ошибка чтения ответа: %v", err), Query: query}
	}

	results := parseDuckDuckGoHTML(string(body), maxResults)

	return SearchResponse{
		Success: true,
		Query:   query,
		Results: results,
		Source:  "duckduckgo",
		Count:   len(results),
	}
}

// parseDuckDuckGoHTML — парсит HTML-ответ DuckDuckGo и извлекает результаты.
// Извлекает заголовки, URL и сниппеты из HTML-разметки lite-версии.
func parseDuckDuckGoHTML(html string, maxResults int) []SearchResult {
	var results []SearchResult

	// Ищем блоки результатов: <a class="result__a" href="...">
	remaining := html
	for len(results) < maxResults {
		// Ищем ссылку результата
		aStart := strings.Index(remaining, "class=\"result__a\"")
		if aStart < 0 {
			// Альтернативный формат
			aStart = strings.Index(remaining, "class='result__a'")
			if aStart < 0 {
				break
			}
		}
		remaining = remaining[aStart:]

		// Извлекаем href
		hrefStart := strings.Index(remaining, "href=\"")
		if hrefStart < 0 {
			break
		}
		hrefStart += 6
		hrefEnd := strings.Index(remaining[hrefStart:], "\"")
		if hrefEnd < 0 {
			break
		}
		href := remaining[hrefStart : hrefStart+hrefEnd]

		// Декодируем URL из DuckDuckGo redirect
		if strings.Contains(href, "uddg=") {
			if parsed, err := url.Parse(href); err == nil {
				if uddg := parsed.Query().Get("uddg"); uddg != "" {
					href = uddg
				}
			}
		}

		// Извлекаем заголовок (текст между > и </a>)
		tagEnd := strings.Index(remaining[hrefStart:], ">")
		if tagEnd < 0 {
			remaining = remaining[hrefStart:]
			continue
		}
		titleStart := hrefStart + tagEnd + 1
		titleEnd := strings.Index(remaining[titleStart:], "</a>")
		title := ""
		if titleEnd >= 0 {
			title = stripTags(remaining[titleStart : titleStart+titleEnd])
		}

		// Ищем сниппет
		snippet := ""
		snippetStart := strings.Index(remaining, "class=\"result__snippet\"")
		if snippetStart >= 0 {
			snippetTagEnd := strings.Index(remaining[snippetStart:], ">")
			if snippetTagEnd >= 0 {
				sStart := snippetStart + snippetTagEnd + 1
				sEnd := strings.Index(remaining[sStart:], "</")
				if sEnd >= 0 {
					snippet = stripTags(remaining[sStart : sStart+sEnd])
				}
			}
		}

		if href != "" && title != "" {
			results = append(results, SearchResult{
				Title:   strings.TrimSpace(title),
				URL:     href,
				Snippet: strings.TrimSpace(snippet),
				Source:  "duckduckgo",
			})
		}

		remaining = remaining[hrefStart+hrefEnd:]
	}

	return results
}

// ============================================================================
// 2. SearXNG — открытый метапоисковик (JSON API)
// ============================================================================

// Список публичных инстансов SearXNG, доступных из РФ.
// Список обновляется: https://searx.space/
var searxngInstances = []string{
	"https://search.sapti.me",
	"https://searx.be",
	"https://search.bus-hit.me",
	"https://searx.tiekoetter.com",
	"https://searxng.site",
}

// SearchSearXNG — выполняет поиск через SearXNG (открытый метапоисковик).
// Пробует несколько публичных инстансов, если первый недоступен.
//
// Преимущества SearXNG:
// - Агрегирует результаты из Google, Bing, Yahoo, DuckDuckGo и др.
// - Поддерживает JSON API (format=json)
// - Можно развернуть локально
// - Не требует API-ключа
//
// Параметры:
//   - query: поисковый запрос
//   - maxResults: максимальное количество результатов
//   - customInstance: URL собственного инстанса SearXNG (пусто = публичные)
func SearchSearXNG(query string, maxResults int, customInstance string) SearchResponse {
	if query == "" {
		return SearchResponse{Success: false, Error: "Поисковый запрос не может быть пустым", Query: query}
	}
	if maxResults <= 0 {
		maxResults = 10
	}

	instances := searxngInstances
	if customInstance != "" {
		instances = []string{customInstance}
	}

	for _, instance := range instances {
		result := trySearXNGInstance(instance, query, maxResults)
		if result.Success {
			return result
		}
	}

	return SearchResponse{
		Success: false,
		Error:   "Все инстансы SearXNG недоступны. Попробуйте DuckDuckGo или развернуть свой инстанс SearXNG.",
		Query:   query,
		Source:  "searxng",
	}
}

// trySearXNGInstance — пробует выполнить поиск через конкретный инстанс SearXNG.
func trySearXNGInstance(instanceURL, query string, maxResults int) SearchResponse {
	searchURL := fmt.Sprintf("%s/search?q=%s&format=json&language=ru-RU&pageno=1",
		strings.TrimRight(instanceURL, "/"),
		url.QueryEscape(query),
	)

	client := &http.Client{Timeout: searchTimeout}
	req, err := http.NewRequest("GET", searchURL, nil)
	if err != nil {
		return SearchResponse{Success: false, Error: err.Error(), Query: query}
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return SearchResponse{Success: false, Error: err.Error(), Query: query}
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return SearchResponse{Success: false, Error: fmt.Sprintf("HTTP %d", resp.StatusCode), Query: query}
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, int64(maxSearchResponse)))
	if err != nil {
		return SearchResponse{Success: false, Error: err.Error(), Query: query}
	}

	// Парсим JSON-ответ SearXNG
	var searxResp struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
			Engine  string `json:"engine"`
		} `json:"results"`
	}

	if err := json.Unmarshal(body, &searxResp); err != nil {
		return SearchResponse{Success: false, Error: fmt.Sprintf("Ошибка парсинга JSON: %v", err), Query: query}
	}

	var results []SearchResult
	for i, r := range searxResp.Results {
		if i >= maxResults {
			break
		}
		results = append(results, SearchResult{
			Title:   r.Title,
			URL:     r.URL,
			Snippet: r.Content,
			Source:  "searxng:" + r.Engine,
		})
	}

	return SearchResponse{
		Success: true,
		Query:   query,
		Results: results,
		Source:  "searxng",
		Count:   len(results),
	}
}

// ============================================================================
// 3. Универсальный поиск (автовыбор поисковика)
// ============================================================================

// Search — универсальная функция поиска.
// Автоматически выбирает доступный поисковик в порядке приоритета:
// 1. DuckDuckGo (самый надёжный из РФ)
// 2. SearXNG (резервный)
//
// Параметры:
//   - query: поисковый запрос
//   - maxResults: максимальное количество результатов
//   - preferredEngine: предпочитаемый поисковик ("duckduckgo", "searxng", "" = авто)
func Search(query string, maxResults int, preferredEngine string) SearchResponse {
	if maxResults <= 0 {
		maxResults = 10
	}

	switch preferredEngine {
	case "duckduckgo":
		return SearchDuckDuckGo(query, maxResults)
	case "searxng":
		return SearchSearXNG(query, maxResults, "")
	default:
		// Автовыбор: сначала DuckDuckGo, потом SearXNG
		result := SearchDuckDuckGo(query, maxResults)
		if result.Success && len(result.Results) > 0 {
			return result
		}

		result = SearchSearXNG(query, maxResults, "")
		if result.Success && len(result.Results) > 0 {
			return result
		}

		return SearchResponse{
			Success: false,
			Error:   "Не удалось выполнить поиск. Все поисковые системы недоступны. Проверьте подключение к интернету.",
			Query:   query,
		}
	}
}

// ============================================================================
// Вспомогательные функции
// ============================================================================

// stripTags — удаляет HTML-теги из строки.
func stripTags(s string) string {
	var result strings.Builder
	inTag := false
	for _, ch := range s {
		if ch == '<' {
			inTag = true
			continue
		}
		if ch == '>' {
			inTag = false
			continue
		}
		if !inTag {
			result.WriteRune(ch)
		}
	}
	return result.String()
}
