package knowledge

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
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

// seedArchive is one compressed source file together with the dataset it
// belongs to.
//
// The directory layout IS the classification: sources live in
// data/<dataset>/<name>.json and compress to gz/<dataset>/<name>.json.zst, so
// the parent directory names the dataset and no source file has to be
// registered anywhere. Every file is a top-level JSON array and every element
// is one row.
type seedArchive struct {
	Dataset string // directory name under the gz root
	Base    string // source JSON name, e.g. "diabetes.json"
	Path    string // archive path on disk
}

// isArchive reports whether a file name is a knowledge archive.
func isArchive(name string) bool {
	ok, err := filepath.Match(archiveGlob, name)
	return err == nil && ok
}

// rows decompresses the archive and splits it into seed rows. The decompressed
// bytes live only in this scope, so a 300MB source JSON is released before its
// rows are embedded — the two callers seed and bake, and neither needs raw.
// The returned errors name the file but not the caller's stage, which the
// callers wrap (see seed.go / bake.go).
func (a seedArchive) rows() ([]KBRow, error) {
	raw, err := decompressFile(a.Path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", a.Path, err)
	}
	rows, err := seedList(raw)
	if err != nil {
		return nil, fmt.Errorf("%s/%s: %w", a.Dataset, a.Base, err)
	}
	return rows, nil
}

// listSeedArchives returns every archive under gzDir/<dataset>/, sorted by
// dataset then file name so builds and logs are reproducible. An empty result
// is an error: a knowledge tree with no archives means the layout is wrong
// (e.g. still the old flat one), and seeding/baking must fail loudly rather
// than insert nothing.
func listSeedArchives(gzDir string) ([]seedArchive, error) {
	dirs, err := os.ReadDir(gzDir)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", gzDir, err)
	}
	var out []seedArchive
	var flat []string
	for _, d := range dirs {
		if !d.IsDir() {
			if isArchive(d.Name()) {
				flat = append(flat, d.Name())
			}
			continue
		}
		ds := d.Name()
		files, err := os.ReadDir(filepath.Join(gzDir, ds))
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", filepath.Join(gzDir, ds), err)
		}
		for _, f := range files {
			if f.IsDir() || !isArchive(f.Name()) {
				continue
			}
			out = append(out, seedArchive{
				Dataset: ds,
				Base:    archiveBaseName(f.Name()),
				Path:    filepath.Join(gzDir, ds, f.Name()),
			})
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no knowledge archives found under %s/<dataset>/", gzDir)
	}
	// A file sitting at the root of gz/ has no directory to name its dataset, so
	// it would be invisible to the layout below. Fail rather than seed nothing.
	if len(flat) > 0 {
		return nil, fmt.Errorf("%s holds %d archive(s) outside a <dataset>/ directory "+
			"(e.g. %s): regenerate the tree with python3 external/make_gz.py",
			gzDir, len(flat), flat[0])
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Dataset != out[j].Dataset {
			return out[i].Dataset < out[j].Dataset
		}
		return out[i].Base < out[j].Base
	})
	return out, nil
}

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
