/*
 * (C) Copyright 2024 Johan Michel PIQUET, France (https://johanpiquet.fr/).
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package httpServer

import (
	"mime/multipart"
	"sync"
	"time"
)

//region Http server

type HttpServer interface {
	GetPort() int
	IsStarted() bool
	Shutdown()
	StartServer() error
	GetHost(hostName string) *HttpHost
	SetStartServerParams(params StartParams)
}

// StartParams will contain information on how
// to configure the server instance to create.
type StartParams struct {
}

// GetHttpServer allows to get the server instance
// listening to the given port. Return nil if no one.
func GetHttpServer(port int) HttpServer {
	gServerByPortMutex.RLock()
	s := gServerByPort[port]
	gServerByPortMutex.RUnlock()
	return s
}

// RegisterServer allows registering a server instance
// in a map allowing to known listened port <--> server.
func RegisterServer(server HttpServer) {
	gServerByPortMutex.Lock()
	gServerByPort[server.GetPort()] = server
	gServerByPortMutex.Unlock()
}

var gServerByPort = make(map[int]HttpServer)
var gServerByPortMutex sync.RWMutex

//endregion

//region Http request

type HttpRequest interface {
	GetMethodName() string
	GetMethodCode() HttpMethod
	GetContentLength() int

	IsBodySend() bool

	GetContentType() string
	SetContentType(contentType string)
	SetHeader(key, value string)

	GetHeaders() map[string]string

	ReturnString(status int, text string)

	GetQueryArgs() ValueSet
	GetPostArgs() ValueSet

	IsMultipartForm() bool
	GetMultipartForm() (*HttpMultiPartForm, error)

	GetCookie(name string) (map[string]any, error)
	GetCookies() (map[string]map[string]any, error)
	SetCookie(key string, value string, cookie HttpCookieOptions) error

	Path() string
	URI() string

	UserAgent() string
	RemoteIP() string

	GetHost() *HttpHost

	Return500ErrorPage(err error)
	Return404UnknownPage()

	SetUnlockMutex(mutex *sync.Mutex)
	UnlockMutex()

	MustStop() bool
	StopRequest()

	GetWildcards() []string
	GetRemainingSegment() []string
}

//endregion

//region Enum HttpMethod

type HttpMethod int

const (
	HttpMethodGET HttpMethod = iota
	HttpMethodPOST
	HttpMethodHEAD
	HttpMethodPUT
	HttpMethodDELETE
	HttpMethodCONNECT
	HttpMethodOPTIONS
	HttpMethodTRACE
	HttpMethodPATCH
)

//endregion

//region Cookies

type HttpCookie interface {
	IsHTTPOnly() bool
	IsSecure() bool
	GetSameSiteType() CookieSameSite
	GetKey() string
	GetDomain() string
	GetValue() string
	GetExpireTime() time.Time
	GetMaxAge() int
}

type HttpCookieOptions struct {
	IsHttpOnly   bool
	IsSecure     bool
	SameSiteType CookieSameSite
	Domaine      string
	ExpireTime   int64
	MaxAge       int
}

type CookieSameSite int

const CookieSameSiteDisabled = CookieSameSite(0)
const CookieSameSiteDefaultMode = CookieSameSite(1)
const CookieSameSiteLaxMode = CookieSameSite(2)
const CookieSameSiteStrictMode = CookieSameSite(3)
const CookieSameSiteNoneMode = CookieSameSite(4)

//endregion

//region Hosts

type HttpHost struct {
	impl         HttpHostImpl
	server       HttpServer
	hostName     string
	urlResolvers []*UrlResolver
}

type HttpHostImpl interface {
	Reset(host *HttpHost)
}

// HttpHostInfos allows designing a host by his hostname and port.
type HttpHostInfos struct {
	HostName string
	Port     int
}

// HttpMiddleware is a function the system can call when a request occurs.
type HttpMiddleware func(call HttpRequest) error

func NewHttpHost(hostName string, server HttpServer, impl HttpHostImpl) *HttpHost {
	res := &HttpHost{
		hostName: hostName,
		server:   server,
	}

	count := int(HttpMethodPATCH)
	res.urlResolvers = make([]*UrlResolver, count)

	for i := 0; i < count; i++ {
		res.urlResolvers[i] = NewUrlResolver()
	}

	return res
}

func (m *HttpHost) Impl() HttpHostImpl {
	return m.impl
}

func (m *HttpHost) Reset() {
	if m.impl != nil {
		m.impl.Reset(m)
	}
}

func (m *HttpHost) GetHostName() string {
	return m.hostName
}

func (m *HttpHost) GetServer() HttpServer {
	return m.server
}

func (m *HttpHost) VERB(verb string, path string, h HttpMiddleware) {
	m.urlResolvers[MethodNameToMethodCode(verb)].Add(path, h, m)
}

func (m *HttpHost) GET(path string, h HttpMiddleware) {
	m.urlResolvers[HttpMethodGET].Add(path, h, m)
}

func (m *HttpHost) POST(path string, h HttpMiddleware) {
	m.urlResolvers[HttpMethodPOST].Add(path, h, m)
}

func (m *HttpHost) HEAD(path string, h HttpMiddleware) {
	m.urlResolvers[HttpMethodHEAD].Add(path, h, m)
}

func (m *HttpHost) PUT(path string, h HttpMiddleware) {
	m.urlResolvers[HttpMethodPUT].Add(path, h, m)
}

func (m *HttpHost) DELETE(path string, h HttpMiddleware) {
	m.urlResolvers[HttpMethodDELETE].Add(path, h, m)
}

func (m *HttpHost) TRACE(path string, h HttpMiddleware) {
	m.urlResolvers[HttpMethodTRACE].Add(path, h, m)
}

func (m *HttpHost) OPTIONS(path string, h HttpMiddleware) {
	m.urlResolvers[HttpMethodOPTIONS].Add(path, h, m)
}

func (m *HttpHost) CONNECT(path string, h HttpMiddleware) {
	m.urlResolvers[HttpMethodCONNECT].Add(path, h, m)
}

func (m *HttpHost) PATCH(path string, h HttpMiddleware) {
	m.urlResolvers[HttpMethodPATCH].Add(path, h, m)
}

func (m *HttpHost) GetUrlResolver(methodCode HttpMethod) *UrlResolver {
	return m.urlResolvers[methodCode]
}

func (m *HttpHost) OnError(req HttpRequest, err error) {
	req.ReturnString(500, "error")
}

func (m *HttpHost) OnNotFound(req HttpRequest) {
	req.ReturnString(404, "not found")
}

//endregion

//region Multipart form

type HttpMultiPartForm struct {
	Values map[string][]string
	Files  map[string][]*multipart.FileHeader
}

//endregion

//region Http return codes

const HttpReturnCode200Ok int = 200
const HttpReturnCode404NotFound int = 404
const HttpReturnCode500ServerError int = 500

//endregion

//region Value set

type ValueSet interface {
	Len() int
	QueryString() []byte
	VisitAll(f func(key, value []byte))

	Has(key string) bool

	GetUfloat(key string) (float64, error)
	GetUfloatOrZero(key string) float64

	GetUint(key string) (int, error)
	GetUintOrZero(key string) int

	GetBool(key string) bool
}

//endregion