package httpServer

import "time"

type FileServer interface {
	Register(host *HttpHost)
	Dispose()

	GetHooks() *FileServerHooks

	VisitCache(func(entry FileServerCacheEntry))
	RemoveExactUri(uri string, data string) error
	RemoveAll()
}

type FileServerRequest struct {
	Call     HttpRequest
	BaseDir  string
	CacheKey string
}

type FileServerHooks struct {
	RewriteCacheKey    FsTargetRewriteCacheKeyF
	CalcCacheEntryData FsTCalcCacheEntryDataF
	RewriteBaseDir     FsTargetRewriteBaseDirKeyF
	OnFileNotFound     FsOnFileNotFoundHookF
	OnTooMuchFiles     FsOnTooMuchFilesHookF
	OnRemoveCacheItem  FsOnRemoveCacheItemHookF
}

type FileServerCacheEntry interface {
	GetHitCount() int
	GetFilePath() string
	GetGzipFilePath() string
	GetFullUri() string
	GetData() string
	GetFileUpdateDate() time.Time
	GetLastRequestedDate() time.Time

	GetContentType() string
	GetContentLength() int
	GetGzipContentLength() int
}

type FsTargetRewriteCacheKeyF func(call HttpRequest) string
type FsTCalcCacheEntryDataF func(call HttpRequest) string
type FsTargetRewriteBaseDirKeyF func(call HttpRequest, defaultBaseDir string) string
type FsOnFileNotFoundHookF func(call HttpRequest, filePath string, data string) error
type FsOnTooMuchFilesHookF func(cache FileServer)
type FsOnRemoveCacheItemHookF func(cacheEntry FileServerCacheEntry, selectData string) (mustRemove bool)
