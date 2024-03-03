package httpServer

type FileCache interface {
	TrySendFile(call HttpRequest, rewrite PathRewriteHandlerF) (bool, error)
}

type PathRewriteHandlerF func(call HttpRequest) (filePath string, dontSend bool, err error)
