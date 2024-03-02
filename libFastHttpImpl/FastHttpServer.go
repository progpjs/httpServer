package libFastHttpImpl

import (
	"crypto/tls"
	"github.com/progpjs/httpServer/v2"
	"github.com/valyala/fasthttp"
	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
	"os"
	"path"
	"strconv"
	"sync"
	"time"
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

	// Setting LogAllErrors to false avoid saturating the console.
	m.server = &fasthttp.Server{
		Handler:      handler,
		LogAllErrors: false,

		// Limit body size to 4Mo.
		MaxRequestBodySize: 4 * 1024 * 1024,

		// Limit to 10sec for receiving the complete request.
		ReadTimeout: time.Second * 10,
	}

	// Use a fake server name for security, making less simple
	// for hacker to known what server technologies is used.
	m.server.Name = "Apache/2.4.38 (Debian)"

	m.server.ErrorHandler = func(ctx *fasthttp.RequestCtx, err error) {
		// Do nothing, avoid saturating the console.
	}

	sPort := ":" + strconv.Itoa(m.port)

	if m.startParams.EnableHttps {
		var customServerStart func() error

		for _, httpsInfo := range m.startParams.Certificates {
			host := m.GetHost(httpsInfo.Hostname)
			host.AllowHttps()

			// Note: use of dev certificate has been remove since it's no more supported by browsers.
			// If you want to create a dev certificate, then use mkcert:
			// https://github.com/FiloSottile/mkcert
			//
			// 1- mkcert -install		--> create the root CA certificat, which is required to create your own test certificate.
			// 2- mkcert localhost

			if httpsInfo.UseLetsEncrypt {
				// Note: LetsEncrypt requires a CAA record on the DNS.
				// It's why it can't be tested on a dev local server.
				// See more: https://letsencrypt.org/docs/caa/
				// Also for an alternative: https://go-acme.github.io/lego/installation/

				certCacheDir := httpsInfo.CertCacheDir

				if !path.IsAbs(certCacheDir) {
					cwd, _ := os.Getwd()
					certCacheDir = path.Join(cwd, certCacheDir)
					_ = os.MkdirAll(certCacheDir, os.ModePerm)
				}

				manager := &autocert.Manager{
					Prompt:     autocert.AcceptTOS,
					HostPolicy: autocert.HostWhitelist(httpsInfo.Hostname),
					Cache:      autocert.DirCache(certCacheDir),
				}

				if m.server.TLSConfig == nil {
					m.server.TLSConfig = &tls.Config{}
				}

				m.server.TLSConfig.GetCertificate = manager.GetCertificate
				m.server.TLSConfig.NextProtos = []string{"http/1.1", acme.ALPNProto}
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

		if customServerStart == nil {
			err := m.server.ListenAndServeTLS(sPort, "", "")
			if err != nil {
				return err
			}
		} else {
			err := customServerStart()
			if err != nil {
				return err
			}
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
