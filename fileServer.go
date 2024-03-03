package httpServer

type FileCache interface {
	TrySendFile(call HttpRequest) (bool, error)
	RemovePath(path string, includeSubPath bool)
	RemoveAll()
}

type FileCacheRequest struct {
	Call     HttpRequest
	BaseDir  string
	CacheKey string
}

type FileServerHooks struct {
	RewriteHook    FsTargetRewriteHookF
	OnFileNotFound FsOnFileNotFoundHookF
	OnTooMuchFiles FsOnTooMuchFilesHookF
}

type FsTargetRewriteHookF func(req *FileCacheRequest) error
type FsOnFileNotFoundHookF func(call HttpRequest, filePath string) error
type FsOnTooMuchFilesHookF func(cache FileCache)
