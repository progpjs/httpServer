package httpServer

type FetchResult interface {
	GetBody() ([]byte, error)
	GetBodyAsString() (string, error)
	GetHeaders() map[string]string
	GetContentLength() int
	GetContentType() string
	GetCookies() (map[string]map[string]any, error)
	GetCookie(name string) (map[string]any, error)
	StatusCode() int

	Dispose()
	StreamBodyToFile(file string) error
}
