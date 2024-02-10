package libFastHttpImpl

import (
	"github.com/progpjs/libHttpServer"
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

func GetFastHttpServer(serverPort int) libHttpServer.HttpServer {
	server := libHttpServer.GetHttpServer(serverPort)

	if server == nil {
		server = NewFastHttpServer(serverPort)
		libHttpServer.RegisterServer(server)
	}

	return server
}
