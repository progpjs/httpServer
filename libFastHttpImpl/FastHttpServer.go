package libFastHttpImpl

import (
	"github.com/progpjs/httpServer/v2"
	"github.com/valyala/fasthttp"
	"strconv"
	"sync"
)

type FastHttpServer struct {
	port        int
	isStarted   bool
	startParams httpServer.StartParams

	hosts      map[string]*httpServer.HttpHost
	hostsMutex sync.Mutex
}

func NewFastHttpServer(port int) *FastHttpServer {
	return &FastHttpServer{
		port:  port,
		hosts: make(map[string]*httpServer.HttpHost),
	}
}

func (m *FastHttpServer) GetPort() int {
	return m.port
}

func (m *FastHttpServer) IsStarted() bool {
	return m.isStarted
}

func (m *FastHttpServer) Shutdown() {
	if !m.isStarted {
		return
	}
}

func (m *FastHttpServer) StartServer() error {
	if m.isStarted {
		return nil
	}

	err := fasthttp.ListenAndServe(":"+strconv.Itoa(m.port), func(fast *fasthttp.RequestCtx) {
		hostName := UnsafeString(fast.Host())
		method := UnsafeString(fast.Method())
		path := UnsafeString(fast.Path())
		methodCode := httpServer.MethodNameToMethodCode(method)

		req := prepareFastHttpRequest(method, methodCode, path, fast)

		host := m.hosts[hostName]
		if host == nil {
			req.Return500ErrorPage(nil)
			return
		}
		//
		req.host = host

		resolver := host.GetUrlResolver(methodCode)
		if resolver == nil {
			host.OnNotFound(req)
			return
		}

		resolvedUrl := resolver.Find(path)
		if resolvedUrl.Target == nil {
			host.OnNotFound(req)
			return
		}

		req.resolvedUrl = resolvedUrl

		if resolvedUrl.Middlewares != nil {
			for _, h := range resolvedUrl.Middlewares {
				err := h.(httpServer.HttpMiddleware)(req)

				if err != nil {
					host.OnError(req, err)
					return
				}

				if req.MustStop() {
					return
				}
			}
		}

		err := resolvedUrl.Target.(httpServer.HttpMiddleware)(req)
		if err != nil {
			host.OnError(req, err)
		}
	})

	if err == nil {
		m.isStarted = true
		return nil
	}

	return err
}

func (m *FastHttpServer) GetHost(hostName string) *httpServer.HttpHost {
	m.hostsMutex.Lock()
	defer m.hostsMutex.Unlock()

	hostName += ":" + strconv.Itoa(m.port)
	host := m.hosts[hostName]

	if host == nil {
		host = httpServer.NewHttpHost(hostName, m, nil)
		m.hosts[hostName] = host
	}

	return host
}

func (m *FastHttpServer) SetStartServerParams(params httpServer.StartParams) {
	m.startParams = params
}
