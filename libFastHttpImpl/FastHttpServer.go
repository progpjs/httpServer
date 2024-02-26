package libFastHttpImpl

import (
	"github.com/progpjs/httpServer/v2"
	"github.com/valyala/fasthttp"
	"os"
	"path"
	"strconv"
	"sync"
)

type FastHttpServer struct {
	port        int
	isStarted   bool
	startParams httpServer.StartParams

	hosts      map[string]*httpServer.HttpHost
	hostsMutex sync.Mutex

	server *fasthttp.Server
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

	handler := func(fast *fasthttp.RequestCtx) {
		hostName := UnsafeString(fast.Host())
		method := UnsafeString(fast.Method())
		rPath := UnsafeString(fast.Path())
		methodCode := httpServer.MethodNameToMethodCode(method)

		req := prepareFastHttpRequest(method, methodCode, rPath, fast)

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

		resolvedUrl := resolver.Find(rPath)
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
	}

	m.server = &fasthttp.Server{Handler: handler}

	sPort := ":" + strconv.Itoa(m.port)

	if m.startParams.EnableHttps {
		for _, httpsInfo := range m.startParams.Certificates {
			if httpsInfo.UseDevCertificate {
				cert, priv, err := fasthttp.GenerateTestCertificate(httpsInfo.Hostname + sPort)
				if err != nil {
					return err
				}

				err = m.server.AppendCertEmbed(cert, priv)
				if err != nil {
					return err
				}
			} else {
				certFilePath := httpsInfo.CertFilePath
				keyFilePath := httpsInfo.KeyFilePath

				if !path.IsAbs(certFilePath) || !path.IsAbs(keyFilePath) {
					cwd, _ := os.Getwd()

					if !path.IsAbs(certFilePath) {
						certFilePath = path.Join(cwd, certFilePath)
					}

					if !path.IsAbs(keyFilePath) {
						keyFilePath = path.Join(cwd, keyFilePath)
					}
				}

				err := m.server.AppendCert(certFilePath, keyFilePath)
				if err != nil {
					return err
				}
			}
		}

		err := m.server.ListenAndServeTLS(sPort, "", "")
		if err != nil {
			return err
		}

	} else {
		err := m.server.ListenAndServe(sPort)
		if err != nil {
			return err
		}
	}

	m.isStarted = true
	return nil
}

func (m *FastHttpServer) GetHost(hostName string) *httpServer.HttpHost {
	m.hostsMutex.Lock()
	defer m.hostsMutex.Unlock()

	if (m.port != 80) && (m.port != 443) {
		hostName += ":" + strconv.Itoa(m.port)
	}

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
