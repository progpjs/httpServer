package libFastHttpImpl

import (
	"github.com/progpjs/httpServer/v2"
	"github.com/valyala/fasthttp"
	"time"
)

func BuildProxyMiddleware(targetUrl string, timeOutInSec int64) (httpServer.HttpMiddleware, error) {
	uri := fasthttp.AcquireURI()
	err := uri.Parse(nil, []byte(targetUrl))
	if err != nil {
		return nil, err
	}

	// https://github.com/valyala/fasthttp/blob/7e1fb718543e4e00f807f081b63ba387570690f4/fasthttpproxy/http.go#L38
	return func(call httpServer.HttpRequest) error {
		// Here we will reuse the current request.
		fastCall := call.(*fastHttpRequest).fast
		//fastCall.Request.SetHost(targetUrl)

		callUri := fastCall.Request.URI()
		uri.CopyTo(callUri)

		err := fasthttp.DoTimeout(&fastCall.Request, &fastCall.Response, time.Second*(time.Duration)(timeOutInSec))
		//err := gFetchHttpClient.Do(&fastCall.Request, resp)
		return err
	}, nil
}

/*
	reader := NewFsFileReader(filePath)

	ctx.SetBodyStream(reader, contentLength)
	hdr.SetContentLength(contentLength)
	hdr.SetContentType(mimeType)

	if contentEncoding != "" {
		hdr.SetContentEncoding(contentEncoding)
	}
*/
