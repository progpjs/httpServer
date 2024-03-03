package libFastHttpImpl

import (
	"errors"
	"github.com/progpjs/httpServer/v2"
	"os"
	"path"
	"strings"
)

func BuildStaticFileServerMiddleware(basePath string, baseDir string, options StaticFileServerOptions) (httpServer.HttpMiddleware, error) {
	baseDir = path.Clean(baseDir)

	if strings.HasPrefix(baseDir, "~") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, errors.New("invalid dir path")
		}

		baseDir = path.Join(homeDir, baseDir[1:])
	}

	stat, err := os.Stat(baseDir)
	if err != nil {
		return nil, errors.New("invalid dir path")
	}

	if !stat.IsDir() {
		return nil, errors.New("invalid dir path")
	}

	return func(call httpServer.HttpRequest) error {
		if options.PathRewriteHandler == nil {
			// Here this don't include arguments (after the ?).
			filePath := call.Path()

			if filePath == "" {
				filePath = "/index.html"
			} else if filePath[len(filePath)-1] == '/' {
				filePath += "index.html"
			}

			if !strings.HasPrefix(filePath, basePath) {
				return errors.New("invalid url")
			}

			filePath = path.Join(baseDir, filePath[len(basePath):])
			return call.SendFile(filePath)
		} else {
			filePath, dontSend, err := options.PathRewriteHandler(call)
			if err != nil {
				return err
			}
			if dontSend {
				return nil
			}

			return call.SendFile(filePath)
		}

	}, nil
}

type StaticFileServerOptions struct {
	PathRewriteHandler func(call httpServer.HttpRequest) (filePath string, dontSend bool, err error)
}
