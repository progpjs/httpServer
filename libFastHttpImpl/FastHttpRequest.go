package libFastHttpImpl

import (
	"bytes"
	"github.com/progpjs/httpServer"
	"github.com/valyala/fasthttp"
	"net"
	"sync"
	"time"
)

type fastHttpRequest struct {
	fast              *fasthttp.RequestCtx
	fastResponse      *fasthttp.Response
	fastRequestHeader *fasthttp.RequestHeader

	path       string
	methodName string
	methodCode httpServer.HttpMethod
	host       *httpServer.HttpHost

	mustStop   bool
	isBodySend bool

	unlockMutex *sync.Mutex
	cookie      fastHttpCookie
	resolvedUrl httpServer.UrlResolverResult

	multiPartForm *httpServer.HttpMultiPartForm
}

func prepareFastHttpRequest(methodName string, methodCode httpServer.HttpMethod, reqPath string, fast *fasthttp.RequestCtx) fastHttpRequest {
	return fastHttpRequest{
		methodName:        methodName,
		methodCode:        methodCode,
		path:              reqPath,
		fast:              fast,
		fastResponse:      &fast.Response,
		fastRequestHeader: &fast.Request.Header,
	}
}

func (m *fastHttpRequest) GetMethodName() string {
	return m.methodName
}

func (m *fastHttpRequest) GetMethodCode() httpServer.HttpMethod {
	return m.methodCode
}

func (m *fastHttpRequest) GetContentLength() int {
	return m.fastRequestHeader.ContentLength()
}

func (m *fastHttpRequest) IsBodySend() bool {
	return m.isBodySend
}

func (m *fastHttpRequest) SetHeader(key, value string) {
	m.fastResponse.Header.Set(key, value)
}

func (m *fastHttpRequest) GetHeaders() map[string]string {
	res := make(map[string]string)

	m.fastRequestHeader.VisitAll(func(key, value []byte) {
		res[UnsafeString(key)] = UnsafeString(value)
	})

	return res
}

func (m *fastHttpRequest) GetContentType() string {
	return UnsafeString(m.fastRequestHeader.ContentType())
}

func (m *fastHttpRequest) SetContentType(contentType string) {
	m.fast.SetContentType(contentType)
}

func (m *fastHttpRequest) ReturnString(status int, text string) {
	if !m.isBodySend {
		m.isBodySend = true

		m.fastResponse.SetStatusCode(status)
		m.fastResponse.AppendBodyString(text)

		m.UnlockMutex()
	}
}

func (m *fastHttpRequest) GetQueryArgs() httpServer.ValueSet {
	r := m.fast.QueryArgs()
	return r
}

func (m *fastHttpRequest) GetPostArgs() httpServer.ValueSet {
	return m.fast.Request.PostArgs()
}

func (m *fastHttpRequest) SetCookie(key string, value string, cookie httpServer.HttpCookieOptions) error {
	var c fasthttp.Cookie

	c.SetKey(key)
	c.SetValue(value)
	c.SetDomain(cookie.Domaine)

	if cookie.MaxAge > 0 {
		c.SetMaxAge(cookie.MaxAge)
	}

	if cookie.ExpireTime > 0 {
		c.SetExpire(time.Unix(cookie.ExpireTime, 0))
	}

	m.fastResponse.Header.SetCookie(&c)

	return nil
}

func (m *fastHttpRequest) GetCookies() (map[string]map[string]any, error) {
	var foundError error
	res := make(map[string]map[string]any)

	m.fastRequestHeader.VisitAllCookie(func(key, value []byte) {
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

func (m *fastHttpRequest) GetCookie(name string) (map[string]any, error) {
	var c = &m.cookie
	err := c.fast.ParseBytes(m.fastRequestHeader.Cookie(name))
	return cookieToJson(c), err
}

var gContentTypeMultipartFormData = []byte("multipart/form-data;")

func (m *fastHttpRequest) IsMultipartForm() bool {
	contentType := m.fastRequestHeader.ContentType()
	return bytes.HasPrefix(contentType, gContentTypeMultipartFormData)
}

func (m *fastHttpRequest) GetMultipartForm() (*httpServer.HttpMultiPartForm, error) {
	if m.multiPartForm != nil {
		return m.multiPartForm, nil
	}

	mpf, err := m.fast.MultipartForm()
	if err != nil {
		return nil, err
	}

	var res = &httpServer.HttpMultiPartForm{
		Values: mpf.Value,
		Files:  mpf.File,
	}

	m.multiPartForm = res
	return res, nil
}

func (m *fastHttpRequest) Path() string {
	return m.path
}

func (m *fastHttpRequest) UserAgent() string {
	return UnsafeString(m.fast.UserAgent())
}

func (m *fastHttpRequest) RemoteIP() string {
	addr := m.fast.RemoteAddr()

	x, ok := addr.(*net.TCPAddr)
	if ok {
		return x.IP.String()
	} else {
		return ""
	}
}

func (m *fastHttpRequest) URI() string {
	return UnsafeString(m.fast.RequestURI())
}

func (m *fastHttpRequest) GetHost() *httpServer.HttpHost {
	return m.host
}

func (m *fastHttpRequest) Return500ErrorPage(err error) {
	m.host.OnError(m, err)
}

func (m *fastHttpRequest) Return404UnknownPage() {
}

func (m *fastHttpRequest) SetUnlockMutex(mutex *sync.Mutex) {
	m.unlockMutex = mutex
}

func (m *fastHttpRequest) UnlockMutex() {
	if m.unlockMutex != nil {
		m.unlockMutex.Unlock()
	}
}

func (m *fastHttpRequest) MustStop() bool {
	return m.mustStop
}

func (m *fastHttpRequest) StopRequest() {
	m.mustStop = true
}

func (m *fastHttpRequest) GetWildcards() []string {
	return m.resolvedUrl.GetWildcards()
}

func (m *fastHttpRequest) GetRemainingSegment() []string {
	return m.resolvedUrl.RemainingSegments
}
