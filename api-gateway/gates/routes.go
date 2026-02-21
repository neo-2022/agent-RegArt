package gates

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

// Route определяет одно прокси-правило.
type Route struct {
	Path    string
	Target  *url.URL
	Methods []string // разрешённые HTTP-методы
}

// NewCustomProxy создает обратный прокси для заданного целевого URL с удалением префикса.
func NewCustomProxy(target *url.URL, prefix string) *httputil.ReverseProxy {
	return &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			// Заменяем схему и хост
			req.URL.Scheme = target.Scheme
			req.URL.Host = target.Host
			// Удаляем префикс из пути
			req.URL.Path = strings.TrimPrefix(req.URL.Path, prefix)
			// Убеждаемся, что путь начинается с "/"
			if !strings.HasPrefix(req.URL.Path, "/") {
				req.URL.Path = "/" + req.URL.Path
			}
			// Также меняем заголовок Host
			req.Host = target.Host
		},
	}
}

// NewProxyWithoutStrip создает обратный прокси, который не изменяет путь запроса.
func NewProxyWithoutStrip(target *url.URL) *httputil.ReverseProxy {
	return &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = target.Scheme
			req.URL.Host = target.Host
			req.Host = target.Host
		},
	}
}

// Routes содержит список маршрутов (будет заполнен в main).
var Routes = []*Route{}
