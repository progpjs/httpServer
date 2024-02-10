package libFastHttpImpl

import (
	"github.com/progpjs/libHttpServer"
	"github.com/valyala/fasthttp"
	"strconv"
	"sync"
)

type fastHttpServer struct {
	port        int
	isStarted   bool
	startParams libHttpServer.HttpServerStartParams

	hosts      map[string]*libHttpServer.HttpHost
	hostsMutex sync.Mutex
}

func NewFastHttpServer(port int) *fastHttpServer {
	return &fastHttpServer{
		port:  port,
		hosts: make(map[string]*libHttpServer.HttpHost),
	}
}

func (m *fastHttpServer) GetPort() int {
	return m.port
}

func (m *fastHttpServer) IsStarted() bool {
	return m.isStarted
}

func (m *fastHttpServer) Shutdown() {
	if !m.isStarted {
		return
	}
}

func (m *fastHttpServer) StartServer() error {
	if m.isStarted {
		return nil
	}

	err := fasthttp.ListenAndServe(":"+strconv.Itoa(m.port), func(fast *fasthttp.RequestCtx) {
		hostName := UnsafeString(fast.Host())
		method := UnsafeString(fast.Method())
		path := UnsafeString(fast.Path())
		methodCode := libHttpServer.MethodNameToMethodCode(method)

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
			host.OnNotFound(&req)
			return
		}

		resolvedUrl := resolver.Find(path)
		if resolvedUrl.Target == nil {
			host.OnNotFound(&req)
			return
		}

		req.resolvedUrl = resolvedUrl

		if resolvedUrl.Middlewares != nil {
			for _, h := range resolvedUrl.Middlewares {
				err := h.(libHttpServer.HttpMiddleware)(&req)

				if err != nil {
					host.OnError(&req, err)
					return
				}

				if req.MustStop() {
					return
				}
			}
		}

		err := resolvedUrl.Target.(libHttpServer.HttpMiddleware)(&req)
		if err != nil {
			host.OnError(&req, err)
		}
	})

	if err == nil {
		m.isStarted = true
		return nil
	}

	return err
}

func (m *fastHttpServer) GetHost(hostName string) *libHttpServer.HttpHost {
	m.hostsMutex.Lock()
	defer m.hostsMutex.Unlock()

	hostName += ":" + strconv.Itoa(m.port)
	host := m.hosts[hostName]

	if host == nil {
		host = libHttpServer.NewHttpHost(hostName, m, nil)
		m.hosts[hostName] = host
	}

	return host
}

func (m *fastHttpServer) SetStartServerParams(params libHttpServer.HttpServerStartParams) {
	m.startParams = params
}
