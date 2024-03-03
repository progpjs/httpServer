package libFastHttpImpl

import (
	"errors"
	"github.com/progpjs/httpServer/v2"
	"github.com/progpjs/progpAPI/v2"
	"github.com/valyala/fasthttp"
	"time"
)

// BuildProxyAsIsMiddleware returns a middleware allowing to proxy a request as-is.
// Here there is not path translation.
func BuildProxyAsIsMiddleware(targetHostName string, timeOutInSec int64) (httpServer.HttpMiddleware, error) {
	uri := fasthttp.AcquireURI()
	err := uri.Parse(nil, []byte(targetHostName))
	if err != nil {
		fasthttp.ReleaseURI(uri)
		return nil, err
	}
	targetHostName = string(uri.Host())
	fasthttp.ReleaseURI(uri)

	// https://github.com/valyala/fasthttp/blob/7e1fb718543e4e00f807f081b63ba387570690f4/fasthttpproxy/http.go#L38
	return func(call httpServer.HttpRequest) error {
		// Here we will reuse the current request.
		fastCall := call.(*fastHttpRequest).fast
		//fastCall.Request.SetHost(targetHostName)

		callUri := fastCall.Request.URI()
		callUri.SetHost(targetHostName)
		//uri.CopyTo(callUri)

		err := fasthttp.DoTimeout(&fastCall.Request, &fastCall.Response, time.Second*(time.Duration)(timeOutInSec))

		if err != nil {
			// Occurs when network error.
			//
			if errors.Is(err, fasthttp.ErrConnectionClosed) {
				// Try again.
				progpAPI.PauseMs(10)
				fastCall.Response.Reset()
				err = fasthttp.DoTimeout(&fastCall.Request, &fastCall.Response, time.Second*(time.Duration)(timeOutInSec))
			}
		}

		return err
	}, nil
}
