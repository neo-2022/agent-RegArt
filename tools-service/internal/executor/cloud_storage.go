// Файл cloud_storage.go — клиент для работы с Яндекс.Диском через REST API.
//
// Яндекс.Диск REST API (https://yandex.ru/dev/disk-api/doc/) предоставляет
// полный доступ к облачному хранилищу пользователя:
//   - Просмотр содержимого папок и информации о файлах
//   - Загрузка файлов на диск и скачивание файлов с диска
//   - Создание и удаление папок
//   - Перемещение и копирование файлов
//   - Получение общей информации о диске (объём, занятое место)
//
// Авторизация:
//
//	OAuth-токен передаётся в заголовке Authorization: OAuth <token>.
//	Токен получается при регистрации приложения в Яндексе и авторизации пользователя.
//
// Базовый URL API: https://cloud-api.yandex.net/v1/disk
//
// Интеграция с AgentCore-NG:
//   - Агенты могут читать и записывать файлы в облако
//   - Через скрепку (📎) в чате можно выбрать файл с Яндекс.Диска
//   - RAG-система может индексировать документы из облака
//   - Пространства (Workspaces) могут хранить файлы в облаке
package executor

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// YandexDiskClient — клиент для работы с REST API Яндекс.Диска.
// Обеспечивает все операции с облачным хранилищем: просмотр, загрузка,
// скачивание, создание папок, удаление, перемещение, копирование.
//
// Поля:
//   - Token: OAuth-токен для авторизации (формат: y0_...)
//   - BaseURL: базовый URL API (по умолчанию https://cloud-api.yandex.net/v1/disk)
//   - HTTP: HTTP-клиент для выполнения запросов
type YandexDiskClient struct {
	Token   string       // OAuth-токен Яндекса
	BaseURL string       // Базовый URL REST API
	HTTP    *http.Client // HTTP-клиент
}

// NewYandexDiskClient — создаёт новый экземпляр клиента Яндекс.Диска.
// Если token пустой, операции будут возвращать ошибку авторизации.
//
// Параметры:
//   - token: OAuth-токен Яндекса
//
// Возвращает: настроенный экземпляр YandexDiskClient
func NewYandexDiskClient(token string) *YandexDiskClient {
	return &YandexDiskClient{
		Token:   token,
		BaseURL: "https://cloud-api.yandex.net/v1/disk",
		HTTP:    &http.Client{},
	}
}

// DiskInfo — общая информация о Яндекс.Диске пользователя.
// Возвращается при запросе GET /v1/disk.
type DiskInfo struct {
	TotalSpace    int64             `json:"total_space"`    // Общий объём диска в байтах
	UsedSpace     int64             `json:"used_space"`     // Занятое место в байтах
	TrashSize     int64             `json:"trash_size"`     // Размер корзины в байтах
	SystemFolders map[string]string `json:"system_folders"` // Системные папки (applications, downloads, etc.)
}

// DiskResource — информация о файле или папке на Яндекс.Диске.
// Используется как для отдельных файлов, так и для элементов в списке.
type DiskResource struct {
	Name     string            `json:"name"`                // Имя файла или папки
	Path     string            `json:"path"`                // Полный путь (disk:/path/to/file)
	Type     string            `json:"type"`                // Тип: "file" или "dir"
	Size     int64             `json:"size,omitempty"`      // Размер в байтах (только для файлов)
	MimeType string            `json:"mime_type,omitempty"` // MIME-тип (только для файлов)
	Created  string            `json:"created"`             // Дата создания (ISO 8601)
	Modified string            `json:"modified"`            // Дата изменения (ISO 8601)
	Embedded *DiskResourceList `json:"_embedded,omitempty"` // Содержимое папки (при запросе папки)
}

// DiskResourceList — список ресурсов внутри папки.
// Поддерживает пагинацию через поля offset и limit.
type DiskResourceList struct {
	Items  []DiskResource `json:"items"`  // Элементы (файлы и папки)
	Limit  int            `json:"limit"`  // Максимальное количество элементов
	Offset int            `json:"offset"` // Смещение от начала списка
	Total  int            `json:"total"`  // Общее количество элементов
	Path   string         `json:"path"`   // Путь к папке
}

// DiskLink — ссылка для загрузки/скачивания файла.
// Возвращается при запросе download или upload URL.
type DiskLink struct {
	Href      string `json:"href"`      // URL для загрузки/скачивания
	Method    string `json:"method"`    // HTTP-метод (GET для скачивания, PUT для загрузки)
	Templated bool   `json:"templated"` // Является ли URL шаблоном
}

// DiskError — структура ошибки API Яндекс.Диска.
type DiskError struct {
	Message     string `json:"message"`     // Текст ошибки
	Description string `json:"description"` // Подробное описание
	Error       string `json:"error"`       // Код ошибки
}

// doRequest — вспомогательный метод для выполнения HTTP-запросов к API Яндекс.Диска.
// Автоматически добавляет заголовок авторизации с OAuth-токеном.
//
// Параметры:
//   - method: HTTP-метод (GET, POST, PUT, DELETE)
//   - url: полный URL запроса
//   - body: тело запроса (может быть nil)
//
// Возвращает:
//   - *http.Response: ответ от API
//   - error: ошибка выполнения запроса или авторизации
func (c *YandexDiskClient) doRequest(method, reqURL string, body io.Reader) (*http.Response, error) {
	if c.Token == "" {
		return nil, fmt.Errorf("токен Яндекс.Диска не настроен")
	}

	req, err := http.NewRequest(method, reqURL, body)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания запроса: %w", err)
	}
	req.Header.Set("Authorization", "OAuth "+c.Token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return c.HTTP.Do(req)
}

// GetDiskInfo — получает общую информацию о Яндекс.Диске пользователя.
// Возвращает объём диска, занятое место, размер корзины и системные папки.
//
// API: GET /v1/disk
//
// Возвращает:
//   - *DiskInfo: информация о диске
//   - error: ошибка запроса или декодирования
func (c *YandexDiskClient) GetDiskInfo() (*DiskInfo, error) {
	resp, err := c.doRequest("GET", c.BaseURL, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, c.parseError(resp)
	}

	var info DiskInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("ошибка декодирования информации о диске: %w", err)
	}
	return &info, nil
}

// ListDir — получает содержимое папки на Яндекс.Диске.
// Возвращает список файлов и подпапок с пагинацией.
//
// API: GET /v1/disk/resources?path=<path>&limit=<limit>&offset=<offset>
//
// Параметры:
//   - path: путь к папке на диске (например, "/" для корня, "/Documents" для папки)
//   - limit: максимальное количество элементов (0 = по умолчанию 20)
//   - offset: смещение для пагинации
//
// Возвращает:
//   - *DiskResource: информация о папке с вложенным списком элементов
//   - error: ошибка запроса, авторизации или если путь не найден
func (c *YandexDiskClient) ListDir(path string, limit, offset int) (*DiskResource, error) {
	if path == "" {
		path = "/"
	}

	reqURL := fmt.Sprintf("%s/resources?path=%s", c.BaseURL, url.QueryEscape(path))
	if limit > 0 {
		reqURL += fmt.Sprintf("&limit=%d", limit)
	}
	if offset > 0 {
		reqURL += fmt.Sprintf("&offset=%d", offset)
	}

	resp, err := c.doRequest("GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, c.parseError(resp)
	}

	var resource DiskResource
	if err := json.NewDecoder(resp.Body).Decode(&resource); err != nil {
		return nil, fmt.Errorf("ошибка декодирования содержимого папки: %w", err)
	}
	return &resource, nil
}

// GetDownloadURL — получает временную ссылку для скачивания файла.
// Ссылка действительна ограниченное время (обычно несколько часов).
//
// API: GET /v1/disk/resources/download?path=<path>
//
// Параметры:
//   - path: путь к файлу на диске
//
// Возвращает:
//   - string: URL для скачивания файла (GET-запрос)
//   - error: ошибка запроса или если файл не найден
func (c *YandexDiskClient) GetDownloadURL(path string) (string, error) {
	reqURL := fmt.Sprintf("%s/resources/download?path=%s", c.BaseURL, url.QueryEscape(path))

	resp, err := c.doRequest("GET", reqURL, nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", c.parseError(resp)
	}

	var link DiskLink
	if err := json.NewDecoder(resp.Body).Decode(&link); err != nil {
		return "", fmt.Errorf("ошибка декодирования ссылки скачивания: %w", err)
	}
	return link.Href, nil
}

// DownloadFile — скачивает содержимое файла с Яндекс.Диска.
// Сначала получает временную ссылку, затем скачивает файл по ней.
//
// Параметры:
//   - path: путь к файлу на диске
//
// Возвращает:
//   - []byte: содержимое файла
//   - error: ошибка скачивания или если файл не найден
func (c *YandexDiskClient) DownloadFile(path string) ([]byte, error) {
	downloadURL, err := c.GetDownloadURL(path)
	if err != nil {
		return nil, fmt.Errorf("ошибка получения ссылки скачивания: %w", err)
	}

	resp, err := c.HTTP.Get(downloadURL)
	if err != nil {
		return nil, fmt.Errorf("ошибка скачивания файла: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ошибка скачивания: статус %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения содержимого файла: %w", err)
	}
	return data, nil
}

// GetUploadURL — получает URL для загрузки файла на Яндекс.Диск.
// Если файл уже существует и overwrite=true, он будет перезаписан.
//
// API: GET /v1/disk/resources/upload?path=<path>&overwrite=<bool>
//
// Параметры:
//   - path: путь назначения на диске (куда загрузить файл)
//   - overwrite: перезаписать существующий файл
//
// Возвращает:
//   - string: URL для загрузки (PUT-запрос с телом файла)
//   - error: ошибка запроса
func (c *YandexDiskClient) GetUploadURL(path string, overwrite bool) (string, error) {
	reqURL := fmt.Sprintf("%s/resources/upload?path=%s&overwrite=%t",
		c.BaseURL, url.QueryEscape(path), overwrite)

	resp, err := c.doRequest("GET", reqURL, nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", c.parseError(resp)
	}

	var link DiskLink
	if err := json.NewDecoder(resp.Body).Decode(&link); err != nil {
		return "", fmt.Errorf("ошибка декодирования ссылки загрузки: %w", err)
	}
	return link.Href, nil
}

// UploadFile — загружает файл на Яндекс.Диск.
// Сначала получает URL для загрузки, затем отправляет содержимое файла.
//
// Параметры:
//   - path: путь назначения на диске
//   - data: содержимое файла
//   - overwrite: перезаписать существующий файл
//
// Возвращает:
//   - error: ошибка загрузки
func (c *YandexDiskClient) UploadFile(path string, data io.Reader, overwrite bool) error {
	uploadURL, err := c.GetUploadURL(path, overwrite)
	if err != nil {
		return fmt.Errorf("ошибка получения ссылки загрузки: %w", err)
	}

	req, err := http.NewRequest("PUT", uploadURL, data)
	if err != nil {
		return fmt.Errorf("ошибка создания запроса загрузки: %w", err)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("ошибка загрузки файла: %w", err)
	}
	defer resp.Body.Close()

	// Успешные коды: 201 (Created) или 202 (Accepted)
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ошибка загрузки: статус %d, тело: %s", resp.StatusCode, string(body))
	}

	return nil
}

// CreateDir — создаёт папку на Яндекс.Диске.
// Если папка уже существует, возвращает ошибку (409 Conflict).
//
// API: PUT /v1/disk/resources?path=<path>
//
// Параметры:
//   - path: путь к создаваемой папке (например, "/Projects/MyApp")
//
// Возвращает:
//   - error: ошибка создания или если папка уже существует
func (c *YandexDiskClient) CreateDir(path string) error {
	reqURL := fmt.Sprintf("%s/resources?path=%s", c.BaseURL, url.QueryEscape(path))

	resp, err := c.doRequest("PUT", reqURL, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Успешный код: 201 (Created)
	if resp.StatusCode != http.StatusCreated {
		return c.parseError(resp)
	}
	return nil
}

// Delete — удаляет файл или папку с Яндекс.Диска.
// По умолчанию перемещает в корзину (permanently=false).
// Если permanently=true, удаляет безвозвратно.
//
// API: DELETE /v1/disk/resources?path=<path>&permanently=<bool>
//
// Параметры:
//   - path: путь к удаляемому ресурсу
//   - permanently: удалить безвозвратно (true) или в корзину (false)
//
// Возвращает:
//   - error: ошибка удаления или если ресурс не найден
func (c *YandexDiskClient) Delete(path string, permanently bool) error {
	reqURL := fmt.Sprintf("%s/resources?path=%s&permanently=%t",
		c.BaseURL, url.QueryEscape(path), permanently)

	resp, err := c.doRequest("DELETE", reqURL, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Успешные коды: 204 (No Content) или 202 (Accepted — для больших удалений)
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusAccepted {
		return c.parseError(resp)
	}
	return nil
}

// Move — перемещает файл или папку на Яндекс.Диске.
// Можно использовать для переименования (перемещение в ту же папку с другим именем).
//
// API: POST /v1/disk/resources/move?from=<from>&path=<to>&overwrite=<bool>
//
// Параметры:
//   - from: исходный путь
//   - to: путь назначения
//   - overwrite: перезаписать, если файл уже существует
//
// Возвращает:
//   - error: ошибка перемещения
func (c *YandexDiskClient) Move(from, to string, overwrite bool) error {
	reqURL := fmt.Sprintf("%s/resources/move?from=%s&path=%s&overwrite=%t",
		c.BaseURL, url.QueryEscape(from), url.QueryEscape(to), overwrite)

	resp, err := c.doRequest("POST", reqURL, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Успешные коды: 201 (Created) или 202 (Accepted)
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusAccepted {
		return c.parseError(resp)
	}
	return nil
}

// Copy — копирует файл или папку на Яндекс.Диске.
//
// API: POST /v1/disk/resources/copy?from=<from>&path=<to>&overwrite=<bool>
//
// Параметры:
//   - from: исходный путь
//   - to: путь назначения копии
//   - overwrite: перезаписать, если файл уже существует
//
// Возвращает:
//   - error: ошибка копирования
func (c *YandexDiskClient) Copy(from, to string, overwrite bool) error {
	reqURL := fmt.Sprintf("%s/resources/copy?from=%s&path=%s&overwrite=%t",
		c.BaseURL, url.QueryEscape(from), url.QueryEscape(to), overwrite)

	resp, err := c.doRequest("POST", reqURL, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusAccepted {
		return c.parseError(resp)
	}
	return nil
}

// Search — поиск файлов на Яндекс.Диске по имени или типу.
// Поддерживает фильтрацию по типу медиа (audio, video, image, document и др.).
//
// API: GET /v1/disk/resources/files?media_type=<type>&limit=<limit>&offset=<offset>
//
// Параметры:
//   - mediaType: тип медиа для фильтрации (пустая строка = все файлы)
//     Допустимые значения: audio, backup, book, compressed, data, development,
//     diskimage, document, encoded, executable, flash, font, image, settings,
//     spreadsheet, text, unknown, video, web
//   - limit: максимальное количество результатов
//   - offset: смещение для пагинации
//
// Возвращает:
//   - []DiskResource: список найденных файлов
//   - error: ошибка поиска
func (c *YandexDiskClient) Search(mediaType string, limit, offset int) ([]DiskResource, error) {
	reqURL := fmt.Sprintf("%s/resources/files?", c.BaseURL)
	params := url.Values{}
	if mediaType != "" {
		params.Set("media_type", mediaType)
	}
	if limit > 0 {
		params.Set("limit", fmt.Sprintf("%d", limit))
	}
	if offset > 0 {
		params.Set("offset", fmt.Sprintf("%d", offset))
	}
	reqURL += params.Encode()

	resp, err := c.doRequest("GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, c.parseError(resp)
	}

	var result struct {
		Items []DiskResource `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("ошибка декодирования результатов поиска: %w", err)
	}
	return result.Items, nil
}

// parseError — извлекает структурированную ошибку из ответа API Яндекс.Диска.
// Пытается декодировать JSON-ответ как DiskError. Если не удаётся —
// возвращает сырое тело ответа как текст ошибки.
func (c *YandexDiskClient) parseError(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)
	var diskErr DiskError
	if err := json.Unmarshal(body, &diskErr); err == nil && diskErr.Message != "" {
		return fmt.Errorf("Яндекс.Диск ошибка %d: %s — %s", resp.StatusCode, diskErr.Error, diskErr.Message)
	}
	return fmt.Errorf("Яндекс.Диск вернул статус %d: %s", resp.StatusCode, string(body))
}

// FormatSize — форматирует размер файла в человекочитаемый вид.
// Используется для отображения в UI и логах.
//
// Примеры:
//   - 500 → "500 B"
//   - 1536 → "1.50 KB"
//   - 1048576 → "1.00 MB"
//   - 1073741824 → "1.00 GB"
func FormatSize(bytes int64) string {
	const (
		kb = 1024
		mb = kb * 1024
		gb = mb * 1024
		tb = gb * 1024
	)
	switch {
	case bytes >= tb:
		return fmt.Sprintf("%.2f TB", float64(bytes)/float64(tb))
	case bytes >= gb:
		return fmt.Sprintf("%.2f GB", float64(bytes)/float64(gb))
	case bytes >= mb:
		return fmt.Sprintf("%.2f MB", float64(bytes)/float64(mb))
	case bytes >= kb:
		return fmt.Sprintf("%.2f KB", float64(bytes)/float64(kb))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

// SimpleDiskItem — упрощённая структура элемента для возврата через API.
// Используется в HTTP-обработчиках для отправки клиенту (web-ui).
// Содержит только необходимые поля без лишних вложенностей.
type SimpleDiskItem struct {
	Name     string `json:"name"`                // Имя файла или папки
	Path     string `json:"path"`                // Полный путь на диске
	Type     string `json:"type"`                // "file" или "dir"
	Size     int64  `json:"size,omitempty"`      // Размер (только для файлов)
	SizeStr  string `json:"size_str,omitempty"`  // Размер в человекочитаемом виде
	MimeType string `json:"mime_type,omitempty"` // MIME-тип (только для файлов)
	Modified string `json:"modified"`            // Дата изменения
}

// ToSimpleItems — конвертирует список DiskResource в упрощённый формат SimpleDiskItem.
// Добавляет человекочитаемый размер файлов (size_str).
// Очищает путь от префикса "disk:" для удобства отображения в UI.
func ToSimpleItems(resources []DiskResource) []SimpleDiskItem {
	items := make([]SimpleDiskItem, len(resources))
	for i, r := range resources {
		path := r.Path
		if strings.HasPrefix(path, "disk:") {
			path = path[5:]
		}
		items[i] = SimpleDiskItem{
			Name:     r.Name,
			Path:     path,
			Type:     r.Type,
			Size:     r.Size,
			SizeStr:  FormatSize(r.Size),
			MimeType: r.MimeType,
			Modified: r.Modified,
		}
	}
	return items
}
