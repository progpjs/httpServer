package libFastHttpImpl

import (
	"compress/gzip"
	"errors"
	"github.com/andybalholm/brotli"
	"github.com/valyala/fasthttp/stackless"
	"io"
	"os"
)

func GzipCompressFile(sourceFile, destFile string, compressionLevel int) error {
	if (compressionLevel != gzip.NoCompression) &&
		(compressionLevel != gzip.BestSpeed) && (compressionLevel != gzip.BestCompression) &&
		(compressionLevel != gzip.DefaultCompression) && (compressionLevel != gzip.HuffmanOnly) {
		return errors.New("invalid compression level")
	}

	sourceFileH, err := os.OpenFile(sourceFile, os.O_RDONLY, os.ModePerm)
	if err != nil {
		return err
	}

	destFileH, err := os.Create(destFile)
	if err != nil {
		return err
	}

	writer := stackless.NewWriter(destFileH, func(w io.Writer) stackless.Writer {
		writer, err := gzip.NewWriterLevel(w, compressionLevel)
		if err != nil {
			return nil
		}
		return writer
	})

	defer func() {
		_ = writer.Close()
	}()

	_, err = io.Copy(writer, sourceFileH)
	if err != nil {
		return err
	}

	err = writer.Flush()
	if err != nil {
		return err
	}

	return nil
}

func BrotliCompressFile(sourceFile, destFile string, compressionLevel int) error {
	if (compressionLevel != brotli.BestSpeed) &&
		(compressionLevel != brotli.BestCompression) &&
		(compressionLevel != brotli.DefaultCompression) {
		return errors.New("invalid compression level")
	}

	sourceFileH, err := os.OpenFile(sourceFile, os.O_RDONLY, os.ModePerm)
	if err != nil {
		return err
	}

	destFileH, err := os.Create(destFile)
	if err != nil {
		return err
	}

	writer := stackless.NewWriter(destFileH, func(w io.Writer) stackless.Writer {
		return brotli.NewWriterLevel(w, compressionLevel)
	})

	defer func() {
		_ = writer.Close()
	}()

	_, err = io.Copy(writer, sourceFileH)
	if err != nil {
		return err
	}

	err = writer.Flush()
	if err != nil {
		return err
	}

	return nil
}
