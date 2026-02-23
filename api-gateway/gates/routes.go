package gates

import (
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"
)

// Route определяет одно прокси-правило.
type Route struct {
	Path    string
	Target  *url.URL
	Methods []string // разрешённые HTTP-методы
}

var longTransport = &http.Transport{
	DialContext:           (&net.Dialer{Timeout: 30 * time.Second}).DialContext,
	ResponseHeaderTimeout: 300 * time.Second,
	IdleConnTimeout:       90 * time.Second,
}

// NewCustomProxy создает обратный прокси для заданного целевого URL с удалением префикса.
func NewCustomProxy(target *url.URL, prefix string) *httputil.ReverseProxy {
	return &httputil.ReverseProxy{
		Transport: longTransport,
		Director: func(req *http.Request) {
			req.URL.Scheme = target.Scheme
			req.URL.Host = target.Host
			req.URL.Path = strings.TrimPrefix(req.URL.Path, prefix)
			if !strings.HasPrefix(req.URL.Path, "/") {
				req.URL.Path = "/" + req.URL.Path
			}
			req.Host = target.Host
		},
	}
}

// NewProxyWithoutStrip создает обратный прокси, который не изменяет путь запроса.
func NewProxyWithoutStrip(target *url.URL) *httputil.ReverseProxy {
	return &httputil.ReverseProxy{
		Transport: longTransport,
		Director: func(req *http.Request) {
			req.URL.Scheme = target.Scheme
			req.URL.Host = target.Host
			req.Host = target.Host
		},
	}
}

// Routes содержит список маршрутов (будет заполнен в main).
var Routes = []*Route{}
