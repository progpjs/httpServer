package libFastHttpImpl

import (
	"errors"
	"github.com/progpjs/httpServer/v2"
	"github.com/valyala/fasthttp"
	"mime"
	"os"
	"path"
	"strings"
	"sync"
	"syscall"
)

//region SimpleFileCache

type SimpleFileCache struct {
	byUrl      map[string]*simpleFileCacheEntry
	byUrlMutex sync.RWMutex

	baseDir  string
	basePath string
}

type simpleFileCacheEntry struct {
	counter int

	filePath string

	mimeType          string
	contentType       string
	contentLength     int
	lastModifiedSince int64
	isBigFile         bool
}

func NewSimpleFileCache(basePath string, baseDir string, _ StaticFileServerOptions) *SimpleFileCache {
	return &SimpleFileCache{
		basePath: basePath,
		baseDir:  baseDir,
		byUrl:    make(map[string]*simpleFileCacheEntry),
	}
}

func (m *SimpleFileCache) TrySendFile(call httpServer.HttpRequest, rewrite httpServer.PathRewriteHandlerF) (bool, error) {
	// Avoid using an unsafe string du to strange behaviors
	// when using the string as map key.
	//
	url := call.URI() + "!"

	m.byUrlMutex.RLock()
	cacheEntry := m.byUrl[url]
	m.byUrlMutex.RUnlock()

	if cacheEntry != nil {
		cacheEntry.counter++

		err := m.sendFile(call, cacheEntry)
		if err != nil {
			return false, err
		}

		return true, nil
	}

	var filePath string

	if rewrite != nil {
		nFilePath, dontSend, err := rewrite(call)
		if err != nil {
			return false, err
		}
		if dontSend {
			return true, nil
		}

		filePath = nFilePath
	} else {
		filePath = call.Path()

		if filePath == "" {
			filePath = "/index.html"
		} else if filePath[len(filePath)-1] == '/' {
			filePath += "index.html"
		}

		if !strings.HasPrefix(filePath, m.basePath) {
			return false, errors.New("invalid url")
		}

		filePath = path.Join(m.baseDir, filePath[len(m.basePath):])
	}

	var err error
	cacheEntry, err = m.addFileToCache(url, filePath)
	if err != nil {
		return false, err
	}

	if cacheEntry == nil {
		return false, nil
	}

	err = m.sendFile(call, cacheEntry)
	if err != nil {
		return true, err
	}

	return true, nil
}

func (m *SimpleFileCache) sendFile(call httpServer.HttpRequest, cacheEntry *simpleFileCacheEntry) error {
	fastRequest := call.(*fastHttpRequest)
	ctx := fastRequest.fast

	hdr := &ctx.Response.Header
	statusCode := 200

	defer func() {
		hdr.SetStatusCode(statusCode)
		fastRequest.unlockMutex()
	}()

	if ctx.IsHead() {
		// Head type request only request information about the file.
		// Here we automatically hands this case, even if the order
		// is to send a file.

		ctx.Response.ResetBody()
		ctx.Response.SkipBody = true
		ctx.SetContentType(cacheEntry.mimeType)
		hdr.SetContentLength(cacheEntry.contentLength)

		/*if cacheEntry.contentEncoding != "" {
			hdr.SetContentEncoding(cacheEntry.contentEncoding)
		}*/
	} else {
		reader := NewFsFileReader(cacheEntry.filePath)

		// "Range" header allows to request only a part of the file.
		// This allows to move the cursor of a video, or resume a big download.
		//
		byteRange := ctx.Request.Header.PeekBytes(gHeaderRange)
		contentLength := cacheEntry.contentLength

		if len(byteRange) > 0 {
			initialContentLength := cacheEntry.contentLength

			startPos, endPos, err := fasthttp.ParseByteRange(byteRange, contentLength)
			diff := endPos - startPos

			// Allows avoiding returning to much content and saturate the network.
			if diff > gBigFileSegmentSize {
				endPos = startPos + gBigFileSegmentSize
				contentLength = gBigFileSegmentSize
			} else {
				contentLength = diff
			}

			if err != nil {
				statusCode = fasthttp.StatusRequestedRangeNotSatisfiable
				return err
			}

			err = reader.SeekTo(int64(startPos), int64(endPos))
			if err != nil {
				statusCode = fasthttp.StatusRequestedRangeNotSatisfiable
				_ = reader.Close()
				return err
			}

			hdr.SetContentRange(startPos, endPos, initialContentLength)

			statusCode = fasthttp.StatusPartialContent
		} else if contentLength >= gBigFileMinSize {
			// If it's a big file don't return the whole file but only his first segment.
			// Allows to avoid to saturate the network.
			//
			statusCode = fasthttp.StatusPartialContent
			contentLength = gBigFileSegmentSize

			err := reader.SeekTo(0, int64(contentLength))
			if err != nil {
				statusCode = fasthttp.StatusRequestedRangeNotSatisfiable
				_ = reader.Close()
				return err
			}
		}

		ctx.SetBodyStream(reader, contentLength)
		hdr.SetContentLength(contentLength)
		hdr.SetContentType(cacheEntry.mimeType)

		/*if cacheEntry.contentEncoding != "" {
			hdr.SetContentEncoding(cacheEntry.contentEncoding)
		}*/
	}

	return nil
}

func (m *SimpleFileCache) addFileToCache(url string, filePath string) (*simpleFileCacheEntry, error) {
	fileStat, err := os.Stat(filePath)
	if err != nil {
		return nil, nil
	}

	if fileStat.IsDir() {
		return nil, errors.New("can't send a directory")
	}

	contentLength := int(fileStat.Size())
	mimeType := mime.TypeByExtension(path.Ext(filePath))

	osInfo, _ := fileStat.Sys().(*syscall.Stat_t)
	lastModifiedSince := osInfo.Mtimespec

	cacheEntry := &simpleFileCacheEntry{
		counter:           1,
		filePath:          filePath,
		mimeType:          mimeType,
		contentLength:     contentLength,
		lastModifiedSince: lastModifiedSince.Sec,
		isBigFile:         contentLength >= gBigFileMinSize,
	}

	m.byUrlMutex.Lock()
	m.byUrl[url] = cacheEntry
	m.byUrlMutex.Unlock()

	return cacheEntry, nil
}

//endregion

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

	// TODO: use options to build the instance
	//       in order to be able to hack it
	//
	fileCache := NewSimpleFileCache(basePath, baseDir, options)

	return func(call httpServer.HttpRequest) error {
		isFound, err := fileCache.TrySendFile(call, options.PathRewriteHandler)

		if isFound {
			if err != nil {
				return err
			}

			return nil
		}

		call.Return404UnknownPage()
		return nil
	}, nil
}

type StaticFileServerOptions struct {
	PathRewriteHandler httpServer.PathRewriteHandlerF
}
