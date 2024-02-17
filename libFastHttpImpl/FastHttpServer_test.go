package libFastHttpImpl

import (
	"github.com/progpjs/httpServer"
	"log"
	"testing"
)

func Test1(test *testing.T) {
	httpServer.RegisterServer(NewFastHttpServer(8000))
	server := httpServer.GetHttpServer(8000)

	host := server.GetHost("localhost")

	host.GET("/", func(call httpServer.HttpRequest) error {
		call.ReturnString(200, "hello world!")
		return nil
	})

	log.Fatal(server.StartServer())
}
