package libFastHttpImpl

import (
	"compress/gzip"
	"errors"
	"github.com/progpjs/httpServer/v2"
	"github.com/valyala/fasthttp"
	"io"
	"io/fs"
	"mime"
	"os"
	"path"
	"strings"
	"sync"
	"syscall"
	"time"
)

//region SimpleFileCache

type SimpleFileCache struct {
	byURI map[string]*simpleFileCacheEntry
	mutex sync.RWMutex

	baseDir  string
	basePath string
}

type simpleFileCacheEntry struct {
	counter int

	filePath      string
	contentLength int

	gzipFilePath      string
	gzipContentLength int

	contentType       string
	lastModifiedSince time.Time
}

func NewSimpleFileCache(basePath string, baseDir string, _ StaticFileServerOptions) *SimpleFileCache {
	return &SimpleFileCache{
		basePath: basePath,
		baseDir:  baseDir,
		byURI:    make(map[string]*simpleFileCacheEntry),
	}
}

func (m *SimpleFileCache) TrySendFile(call httpServer.HttpRequest, rewrite httpServer.PathRewriteHandlerF) (bool, error) {
	// Avoid using an unsafe string du to strange behaviors
	// when using the string as map key.
	//
	cacheKey := call.URI() + "!"

	m.mutex.RLock()
	cacheEntry := m.byURI[cacheKey]
	m.mutex.RUnlock()

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
			return false, errors.New("invalid cacheKey")
		}

		filePath = path.Join(m.baseDir, filePath[len(m.basePath):])
	}

	var err error
	cacheEntry, err = m.addFileToCache(cacheKey, filePath)
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

	if !ctx.IfModifiedSince(cacheEntry.lastModifiedSince) {
		ctx.NotModified()
		return nil
	}

	hdr := &ctx.Response.Header
	hdr.SetLastModified(cacheEntry.lastModifiedSince)

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
		ctx.SetContentType(cacheEntry.contentType)

		if cacheEntry.gzipFilePath == "" {
			hdr.SetContentLength(cacheEntry.contentLength)
		} else {
			hdr.SetContentLength(cacheEntry.gzipContentLength)
			hdr.SetContentEncodingBytes(gGzipContentEncoding)
		}
	} else {
		var filePath string
		var contentLength int
		var isGip bool

		if cacheEntry.gzipFilePath == "" {
			filePath = cacheEntry.filePath
			contentLength = cacheEntry.contentLength
		} else {
			filePath = cacheEntry.gzipFilePath
			contentLength = cacheEntry.gzipContentLength
			isGip = true
		}

		reader := NewFsFileReader(filePath)

		// "Range" header allows to request only a part of the file.
		// This allows to move the cursor of a video, or resume a big download.
		//
		byteRange := ctx.Request.Header.PeekBytes(gHeaderRange)

		if len(byteRange) > 0 {
			initialContentLength := contentLength

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
		hdr.SetContentType(cacheEntry.contentType)

		if isGip {
			hdr.SetContentEncodingBytes(gGzipContentEncoding)
		}
	}

	return nil
}

func (m *SimpleFileCache) addFileToCache(uri string, filePath string) (*simpleFileCacheEntry, error) {
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
		contentType:       mimeType,
		contentLength:     contentLength,
		lastModifiedSince: time.Unix(lastModifiedSince.Sec, lastModifiedSince.Nsec),
	}

	m.mutex.Lock()
	m.byURI[uri] = cacheEntry
	m.mutex.Unlock()

	gzipFilePath := filePath + ".gzip"

	if contentLength < gDontCompressOverSize {
		// We always rebuild the gzip version in order to prevent errors
		// where the gzip version is ko.
		//
		err = GzipCompressFile(filePath, gzipFilePath, gzip.BestCompression)

		// Try again after a pause.
		//
		if err != nil {
			time.Sleep(time.Millisecond * 250)
			err = GzipCompressFile(filePath, gzipFilePath, gzip.BestCompression)

			if err != nil {
				return nil, err
			}
		}

		var stat os.FileInfo
		stat, err = os.Stat(gzipFilePath)
		if err != nil {
			return nil, err
		}

		cacheEntry.gzipFilePath = gzipFilePath
		cacheEntry.gzipContentLength = int(stat.Size())
	}

	return cacheEntry, nil
}

//endregion

//region FsFileReader

// FsFileReader allows to read and stream a small file.
type FsFileReader struct {
	filePath string
	reader   io.Reader
	lr       *io.LimitedReader
	file     fs.File
}

func NewFsFileReader(filePath string) *FsFileReader {
	return &FsFileReader{filePath: filePath}
}

func (m *FsFileReader) SeekTo(begin, end int64) error {
	if m.file == nil {
		err := m.open()
		if err != nil {
			return err
		}
	}

	seeker := m.file.(io.Seeker)

	_, err := seeker.Seek(begin, io.SeekStart)
	if err != nil {
		return err
	}

	vMax := end - begin
	if vMax < 0 {
		vMax = 0
	}

	m.reader = io.LimitReader(m.reader, vMax)

	return nil
}

func (m *FsFileReader) Close() error {
	if m.file != nil {
		f := m.file
		m.file = nil
		return f.Close()
	}

	return nil
}

func (m *FsFileReader) open() error {
	var err error
	m.file, err = os.OpenFile(m.filePath, os.O_RDONLY, os.ModePerm)
	if err != nil {
		return err
	}

	m.reader = m.file
	return nil
}

func (m *FsFileReader) Read(buffer []byte) (int, error) {
	if m.file == nil {
		err := m.open()
		if err != nil {
			return 0, err
		}
	}

	count, err := m.reader.Read(buffer)
	if err != nil {
		_ = m.Close()
		return count, err
	}

	if count == 0 {
		_ = m.Close()
	}

	return count, nil
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

var gGzipContentEncoding = []byte("gzip")
var gHeaderRange = []byte("Range")

// gBigFileSegmentSize allows to limit the size of the data send.
// Without that video are entirely send each time, even when the read cursor is moved.
// A little value result in a lot of request, but few data send.
const gBigFileSegmentSize = 1024 * 1024 * 1 // 1Mo

// gBigFileMinSize allows from which size a file is considered a big file.
// Big files allows to seek content position, it's the only difference.
const gBigFileMinSize = gBigFileSegmentSize

const gDontCompressOverSize = 1024 * 1024 * 50 // 50Mo
