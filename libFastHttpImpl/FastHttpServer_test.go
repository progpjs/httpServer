package libFastHttpImpl

import (
	"github.com/progpjs/libHttpServer"
	"log"
	"testing"
)

func Test1(test *testing.T) {
	libHttpServer.RegisterServer(NewFastHttpServer(8000))
	server := libHttpServer.GetHttpServer(8000)

	host := server.GetHost("localhost")

	host.GET("/", func(call libHttpServer.HttpRequest) error {
		call.ReturnString(200, "hello world!")
		return nil
	})

	log.Fatal(server.StartServer())
}
