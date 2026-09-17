package knowledge

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/klauspost/compress/zstd"
)

// Knowledge archive format:
//
//	gzip  (legacy, extension .gz)  — magic 1f 8b
//	zstd  (current, extension .zst) — magic 28 b5 2f fd
//
// The loader auto-detects by magic bytes so both formats are readable
// (backward compatibility with the pre-2026-08-30 gzip archives).
// zstd level 19 gives ~38% smaller files than gzip -9 on the QA corpora.

// archiveGlob matches both legacy gzip and current zstd knowledge archives.
const archiveGlob = "*.json.*z*" // covers .json.gz and .json.zst

// archiveBaseName strips the archive extension, returning the source JSON name
// (e.g. "diabetes.json" from "diabetes.json.zst" or "diabetes.json.gz").
func archiveBaseName(path string) string {
	base := filepath.Base(path)
	for _, ext := range []string{".json.zst", ".json.gz", ".gz", ".zst"} {
		if strings.HasSuffix(base, ext) {
			return base[:len(base)-len(ext)] + ".json"
		}
	}
	return base
}

// readArchiveFile reads a knowledge archive file (gzip or zstd, auto-detected).
func readArchiveFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return decompressArchive(data)
}

// decompressArchive decompresses knowledge bytes, auto-detecting the format.
func decompressArchive(data []byte) ([]byte, error) {
	if len(data) >= 4 && bytes.Equal(data[0:4], []byte{0x28, 0xb5, 0x2f, 0xfd}) {
		// zstd magic
		dec, err := zstd.NewReader(nil, zstd.WithDecoderConcurrency(1))
		if err != nil {
			return nil, fmt.Errorf("creating zstd decoder: %w", err)
		}
		defer dec.Close()
		out, err := dec.DecodeAll(data, nil)
		if err != nil {
			return nil, fmt.Errorf("zstd decompress: %w", err)
		}
		return out, nil
	}
	// gzip (legacy)
	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	return io.ReadAll(zr)
}

// decompressFile reads and decompresses a knowledge archive by path.
// Kept for callers that pass an explicit path (seed.go, bake.go).
func decompressFile(path string) ([]byte, error) {
	return readArchiveFile(path)
}
