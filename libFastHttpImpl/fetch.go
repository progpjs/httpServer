package libFastHttpImpl

import (
	"bufio"
	"github.com/progpjs/httpServer/v2"
	"github.com/valyala/fasthttp"
	"os"
	"path"
	"strings"
	"sync"
	"time"
)

var gHttpClient *fasthttp.Client
var gHttpClientMutex sync.Mutex

func initHttpClient() {
	gHttpClientMutex.Lock()
	defer gHttpClientMutex.Unlock()

	if gHttpClient != nil {
		return
	}

	// It's avoid to wait an infinite time
	// while allowing time for long processing requests.
	//
	readTimeout := time.Minute * 3
	writeTimeout := time.Minute * 3

	maxIdleConnDuration := time.Hour * 1

	gHttpClient = &fasthttp.Client{
		ReadTimeout:         readTimeout,
		WriteTimeout:        writeTimeout,
		MaxIdleConnDuration: maxIdleConnDuration,

		// Avoid sending the default User-Agent which is "fasthttp".
		// Is only useful if no user agent is explicitly set.
		NoDefaultUserAgentHeader:      true,
		DisableHeaderNamesNormalizing: true,
		DisablePathNormalizing:        true,

		Dial: (&fasthttp.TCPDialer{
			Concurrency: 4096,

			// The cache allow to keep info url <--> ip.
			// Here keep it on hour.
			//
			DNSCacheDuration: time.Hour,
		}).Dial,

		// Limit read buffer size to 3Mo.
		// Shorted size must impact header.
		ReadBufferSize: 1024 * 1024 * 3,

		// Always stream response. It's a little slower
		// but it allows using less memory and avoid difficulties
		// when the response body has a big size (ex: downloading a video)
		//
		StreamResponseBody: true,
	}
}

func Fetch(url string, methodName string, options FetchOptions) (httpServer.FetchResult, error) {
	if gHttpClient == nil {
		initHttpClient()
	}

	portIdx := strings.Index(url, ":")
	if portIdx == -1 {
		if strings.HasPrefix(url, "https://") {
			url += ":443"
		} else {
			url += ":80"
		}
	}

	req := fasthttp.AcquireRequest()
	defer fasthttp.ReleaseRequest(req)

	// Set the request body if exists.
	// Allows to send POST data for example.
	//
	if options.BodyStreamWriter != nil {
		req.SetBodyStreamWriter(options.BodyStreamWriter)
	} else if options.Body != nil {
		req.SetBody(options.Body)
	}

	req.SetRequestURI(url)
	req.Header.SetMethod(methodName)

	if options.SendHeaders != nil {
		for k, v := range options.SendHeaders {
			req.Header.Set(k, v)
		}
	}

	if options.SendCookies != nil {
		for k, v := range options.SendCookies {
			req.Header.SetCookie(k, v)
		}
	}

	resp := fasthttp.AcquireResponse()
	resp.SkipBody = options.SkipBody

	err := gHttpClient.Do(req, resp)
	if err != nil {
		fasthttp.ReleaseResponse(resp)
		return nil, err
	}

	return &fetchResultImpl{resp: resp}, nil
}

type FetchOptions struct {
	SendHeaders map[string]string
	SendCookies map[string]string
	SkipBody    bool

	Body             []byte
	BodyStreamWriter func(w *bufio.Writer)
}

type fetchResultImpl struct {
	resp   *fasthttp.Response
	cookie fastHttpCookie
}

func (m *fetchResultImpl) StatusCode() int {
	return m.resp.StatusCode()
}

func (m *fetchResultImpl) StreamBodyToFile(filePath string) error {
	_ = os.MkdirAll(path.Dir(filePath), os.ModePerm)

	fileH, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer func() {
		_ = fileH.Close()
	}()

	_, err = m.resp.WriteTo(fileH)
	return err
}

func (m *fetchResultImpl) Dispose() {
	if m.resp != nil {
		fasthttp.ReleaseResponse(m.resp)
		m.resp = nil
	}
}

func (m *fetchResultImpl) GetBody() ([]byte, error) {
	return m.resp.BodyUncompressed()
}

func (m *fetchResultImpl) GetBodyAsString() (string, error) {
	b, err := m.resp.BodyUncompressed()
	if err != nil {
		return "", err
	}

	return string(b), nil
}

func (m *fetchResultImpl) GetContentLength() int {
	return m.resp.Header.ContentLength()
}

func (m *fetchResultImpl) GetHeaders() map[string]string {
	res := make(map[string]string)

	m.resp.Header.VisitAll(func(key, value []byte) {
		res[UnsafeString(key)] = UnsafeString(value)
	})

	return res
}

func (m *fetchResultImpl) GetContentType() string {
	return UnsafeString(m.resp.Header.ContentType())
}

func (m *fetchResultImpl) GetCookies() (map[string]map[string]any, error) {
	var foundError error
	res := make(map[string]map[string]any)

	m.resp.Header.VisitAllCookie(func(key, value []byte) {
		if len(value) == 0 {
			return
		}

		var c = &m.cookie

		err := c.fast.ParseBytes(value)

		if err != nil {
			foundError = err
		} else {
			res[UnsafeString(key)] = cookieToJson(c)
		}
	})

	return res, foundError
}

func (m *fetchResultImpl) GetCookie(name string) (map[string]any, error) {
	var c = &m.cookie
	cookieValue := m.resp.Header.PeekCookie(name)
	if cookieValue == nil {
		return nil, nil
	}

	err := c.fast.ParseBytes(cookieValue)
	return cookieToJson(c), err
}
