package httpServer

import "time"

type FileCache interface {
	TrySendFile(call HttpRequest) (bool, error)
	RemoveExactUri(uri string, data string) error
	RemoveAll()
	VisitEntries(func(entry FileCacheEntry))
}

type FileCacheRequest struct {
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

type FileCacheEntry interface {
	GetFilePath() string
	GetGzipFilePath() string
	GetFullUri() string
	GetData() string
	GetFileUpdateDate() time.Time
	GetLastRequestedDate() time.Time
}

type FsTargetRewriteCacheKeyF func(call HttpRequest) string
type FsTCalcCacheEntryDataF func(call HttpRequest) string
type FsTargetRewriteBaseDirKeyF func(call HttpRequest, defaultBaseDir string) string
type FsOnFileNotFoundHookF func(call HttpRequest, filePath string) error
type FsOnTooMuchFilesHookF func(cache FileCache)
type FsOnRemoveCacheItemHookF func(cacheEntry FileCacheEntry, data string) (mustRemove bool)
