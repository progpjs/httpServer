package libFastHttpImpl

import (
	"github.com/progpjs/httpServer"
	"unsafe"
)

// UnsafeString returns a string pointer without allocation
func UnsafeString(b []byte) string {
	return unsafe.String(unsafe.SliceData(b), len(b))
}

// UnsafeBytes returns a byte pointer without allocation.
func UnsafeBytes(s string) []byte {
	return unsafe.Slice(unsafe.StringData(s), len(s))
}

func GetFastHttpServer(serverPort int) httpServer.HttpServer {
	server := httpServer.GetHttpServer(serverPort)

	if server == nil {
		server = NewFastHttpServer(serverPort)
		httpServer.RegisterServer(server)
	}

	return server
}
