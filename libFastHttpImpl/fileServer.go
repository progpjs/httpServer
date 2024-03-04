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

	hooks *httpServer.FileServerHooks

	fileCount    int
	maxFileCount int
}

var gFsEmptyHooks = &httpServer.FileServerHooks{}

func NewSimpleFileCache(basePath string, baseDir string, options StaticFileServerOptions) *SimpleFileCache {
	m := &SimpleFileCache{
		basePath: basePath,
		baseDir:  baseDir,
		byURI:    make(map[string]*simpleFileCacheEntry),
		hooks:    options.Hooks,
	}

	if m.hooks == nil {
		m.hooks = gFsEmptyHooks
	}
	return m
}

func (m *SimpleFileCache) VisitEntries(f func(entry httpServer.FileCacheEntry)) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	for _, entry := range m.byURI {
		f(entry)
	}
}

func (m *SimpleFileCache) RemoveAll() {
	m.mutex.Lock()
	m.byURI = make(map[string]*simpleFileCacheEntry)
	m.mutex.Unlock()
}

func (m *SimpleFileCache) RemoveExactUri(uri string, data string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	var toRemove []string

	for cacheKey, entry := range m.byURI {
		if entry.uri == uri {
			toRemove = append(toRemove, cacheKey)
		}
	}

	if m.hooks.OnRemoveCacheItem == nil {
		for _, key := range toRemove {
			cacheEntry := m.byURI[key]
			if cacheEntry.gzipFilePath != "" {
				_ = os.Remove(cacheEntry.gzipFilePath)
			}

			delete(m.byURI, key)
		}
	} else {
		for _, key := range toRemove {
			if m.hooks.OnRemoveCacheItem(m.byURI[key], data) {
				delete(m.byURI, key)
			}
		}
	}

	return nil
}

func (m *SimpleFileCache) TrySendFile(call httpServer.HttpRequest) (bool, error) {
	var cacheKey string

	if m.hooks.RewriteCacheKey != nil {
		// Warning: cache key must be a true string here and not an unsafeString get from bytes.
		// Only true string works correctly with map index.
		//
		cacheKey = m.hooks.RewriteCacheKey(call)
	} else {
		cacheKey = string(call.URI().UriPath())
	}

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

	baseDir := m.baseDir

	if m.hooks.RewriteBaseDir != nil {
		baseDir = m.hooks.RewriteBaseDir(call, baseDir)
	}

	filePath := call.Path()

	if filePath == "" {
		filePath = "/index.html"
	} else if filePath[len(filePath)-1] == '/' {
		filePath += "index.html"
	}

	filePath = path.Join(baseDir, filePath[len(m.basePath):])
	if !strings.HasPrefix(filePath, baseDir) {
		return false, errors.New("invalid cacheKey")
	}

	var err error
	cacheEntry, err = m.addFileToCache(call, cacheKey, filePath)
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

	cacheEntry.lastRequestedDate = time.Now()
	if !ctx.IfModifiedSince(cacheEntry.fileUpdateDate) {
		ctx.NotModified()
		return nil
	}

	hdr := &ctx.Response.Header
	hdr.SetLastModified(cacheEntry.fileUpdateDate)

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

func (m *SimpleFileCache) addFileToCache(call httpServer.HttpRequest, cacheKey string, filePath string) (*simpleFileCacheEntry, error) {
	fileStat, err := os.Stat(filePath)
	if err != nil {
		if m.hooks.OnFileNotFound != nil {
			err = m.hooks.OnFileNotFound(call, filePath)
			if err != nil {
				return nil, err
			}
		}

		fileStat, err = os.Stat(filePath)

		if err != nil {
			return nil, nil
		}
	}

	if fileStat.IsDir() {
		return nil, errors.New("can't send a directory")
	}

	contentLength := int(fileStat.Size())
	mimeType := mime.TypeByExtension(path.Ext(filePath))

	osInfo, _ := fileStat.Sys().(*syscall.Stat_t)
	lastModifiedSince := osInfo.Mtimespec

	var data string
	if m.hooks.CalcCacheEntryData != nil {
		data = m.hooks.CalcCacheEntryData(call)
	}

	cacheEntry := &simpleFileCacheEntry{
		counter:        1,
		data:           data,
		uri:            call.FullURI(),
		filePath:       filePath,
		contentType:    mimeType,
		contentLength:  contentLength,
		fileUpdateDate: time.Unix(lastModifiedSince.Sec, lastModifiedSince.Nsec),
	}

	m.mutex.Lock()
	m.byURI[cacheKey] = cacheEntry
	counter := m.fileCount
	m.fileCount++

	if (counter > m.maxFileCount) && (m.hooks.OnTooMuchFiles != nil) {
		m.hooks.OnTooMuchFiles(m)
	}

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

//region simpleFileCacheEntry

type simpleFileCacheEntry struct {
	counter int

	uri           string
	filePath      string
	contentLength int

	gzipFilePath      string
	gzipContentLength int

	contentType       string
	fileUpdateDate    time.Time
	lastRequestedDate time.Time

	// Allow to set data to a cache entry in order to seperated two entry
	// with the same uri. For example on entry for a special user and another for
	// other users.
	data string
}

func (m *simpleFileCacheEntry) GetFilePath() string {
	return m.filePath
}

func (m *simpleFileCacheEntry) GetGzipFilePath() string {
	return m.gzipFilePath
}

func (m *simpleFileCacheEntry) GetFullUri() string {
	return m.uri
}

func (m *simpleFileCacheEntry) GetData() string {
	return m.data
}

func (m *simpleFileCacheEntry) GetFileUpdateDate() time.Time {
	return m.fileUpdateDate
}

func (m *simpleFileCacheEntry) GetLastRequestedDate() time.Time {
	return m.lastRequestedDate
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
		isFound, err := fileCache.TrySendFile(call)

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
	Hooks *httpServer.FileServerHooks
}

// gBigFileSegmentSize allows to limit the size of the data send.
// Without that video are entirely send each time, even when the read cursor is moved.
// A little value result in a lot of request, but few data send.
const gBigFileSegmentSize = 1024 * 1024 * 1 // 1Mo

// gBigFileMinSize allows from which size a file is considered a big file.
// Big files allows to seek content position, it's the only difference.
const gBigFileMinSize = gBigFileSegmentSize

const gDontCompressOverSize = 1024 * 1024 * 50 // 50Mo

var gGzipContentEncoding = []byte("gzip")
var gHeaderRange = []byte("Range")
