package libFastHttpImpl

import (
	"bytes"
	"errors"
	"github.com/progpjs/httpServer/v2"
	"github.com/valyala/fasthttp"
	"mime"
	"net"
	"os"
	"path"
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

	unlockMutex_ sync.Mutex
	cookie       fastHttpCookie
	resolvedUrl  httpServer.UrlResolverResult

	multiPartForm *httpServer.HttpMultiPartForm
	uri           *fasthttp.URI
}

func prepareFastHttpRequest(methodName string, methodCode httpServer.HttpMethod, reqPath string, fast *fasthttp.RequestCtx) *fastHttpRequest {
	m := fastHttpRequest{
		methodName:        methodName,
		methodCode:        methodCode,
		path:              reqPath,
		fast:              fast,
		fastResponse:      &fast.Response,
		fastRequestHeader: &fast.Request.Header,
	}

	m.unlockMutex_.Lock()
	return &m
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

		m.unlockMutex()
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
	c.SetDomain(cookie.Domain)

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

func (m *fastHttpRequest) URI() httpServer.UriReader {
	return m
}

func (m *fastHttpRequest) FullURI() string {
	return string(m.uri.FullURI())
}

func (m *fastHttpRequest) UriPath() []byte {
	if m.uri == nil {
		m.uri = m.fast.Request.URI()
	}

	return m.uri.Path()
}

func (m *fastHttpRequest) UriArgs(f func(key, value []byte)) {
	if m.uri == nil {
		m.uri = m.fast.Request.URI()
	}

	m.uri.QueryArgs().VisitAll(f)
}

func (m *fastHttpRequest) UriQueryString() []byte {
	if m.uri == nil {
		m.uri = m.fast.Request.URI()
	}

	return m.uri.QueryString()
}

func (m *fastHttpRequest) UriScheme() []byte {
	if m.uri == nil {
		m.uri = m.fast.Request.URI()
	}

	return m.uri.Scheme()
}

func (m *fastHttpRequest) UriHost() []byte {
	if m.uri == nil {
		m.uri = m.fast.Request.URI()
	}

	return m.uri.Host()
}

func (m *fastHttpRequest) GetHost() *httpServer.HttpHost {
	return m.host
}

func (m *fastHttpRequest) Return500ErrorPage(err error) {
	m.host.OnError(m, err)
}

func (m *fastHttpRequest) WaitResponse() {
	m.unlockMutex_.Lock()
}

func (m *fastHttpRequest) Return404UnknownPage() {
	m.host.OnNotFound(m)
}

func (m *fastHttpRequest) unlockMutex() {
	m.unlockMutex_.Unlock()
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

func (m *fastHttpRequest) SendFile(filePath string) error {
	if m.isBodySend {
		return nil
	}

	defer m.unlockMutex()
	m.isBodySend = true

	m.fast.SendFile(filePath)
	return nil
}

func (m *fastHttpRequest) SendFileAsIs(filePath string, mimeType string, contentEncoding string) error {
	if m.isBodySend {
		return nil
	}
	statusCode := 200
	hdr := &m.fast.Response.Header

	defer func() {
		hdr.SetStatusCode(statusCode)
		m.unlockMutex()
	}()

	m.isBodySend = true

	ctx := m.fast

	fileStat, err := os.Stat(filePath)
	if err != nil {
		return errors.New("file not found")
	}

	if fileStat.IsDir() {
		return errors.New("can't send a directory")
	}

	contentLength := int(fileStat.Size())

	if mimeType == "" {
		mimeType = mime.TypeByExtension(path.Ext(filePath))
	}

	lastModifiedSince := getUpdateDate(fileStat)
	uLastModified := time.Unix(lastModifiedSince.Sec, lastModifiedSince.Nsec)

	if !ctx.IfModifiedSince(uLastModified) {
		ctx.NotModified()
		return nil
	}

	hdr.SetLastModified(uLastModified)

	if ctx.IsHead() {
		// Head type request only request information about the file.
		// Here we automatically hands this case, even if the order
		// is to send a file.

		ctx.Response.ResetBody()
		ctx.Response.SkipBody = true
		ctx.SetContentType(mimeType)
		hdr.SetContentLength(contentLength)

		if contentEncoding != "" {
			hdr.SetContentEncoding(contentEncoding)
		}
	} else {
		reader := NewFsFileReader(filePath)

		// "Range" header allows to request only a part of the file.
		// This allows to move the cursor of a video, or resume a big download.
		//
		byteRange := ctx.Request.Header.PeekBytes(gHeaderRange)

		if len(byteRange) > 0 {
			startPos, endPos, err := fasthttp.ParseByteRange(byteRange, contentLength)
			diff := endPos - startPos

			initialContentLength := contentLength

			// Allows avoiding returning to much content and saturate the network.
			if diff > gBigFileSegmentSize {
				endPos = startPos + gBigFileSegmentSize
				contentLength = gBigFileSegmentSize
			} else {
				contentLength = diff
			}

			if err != nil {
				statusCode = fasthttp.StatusRequestedRangeNotSatisfiable
				return err
			}

			err = reader.SeekTo(int64(startPos), int64(endPos))
			if err != nil {
				statusCode = fasthttp.StatusRequestedRangeNotSatisfiable
				_ = reader.Close()
				return err
			}

			hdr.SetContentRange(startPos, endPos, initialContentLength)

			statusCode = fasthttp.StatusPartialContent
		} else if contentLength >= gBigFileMinSize {
			// If it's a big file don't return the whole file but only his first segment.
			// Allows to avoid to saturate the network.
			//
			statusCode = fasthttp.StatusPartialContent
			contentLength = gBigFileSegmentSize

			err = reader.SeekTo(0, int64(contentLength))
			if err != nil {
				statusCode = fasthttp.StatusRequestedRangeNotSatisfiable
				_ = reader.Close()
				return err
			}
		}

		ctx.SetBodyStream(reader, contentLength)
		hdr.SetContentLength(contentLength)
		hdr.SetContentType(mimeType)

		if contentEncoding != "" {
			hdr.SetContentEncoding(contentEncoding)
		}
	}

	return nil
}
