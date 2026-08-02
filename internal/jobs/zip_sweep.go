package jobs

import (
	"archive/zip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// WriteZip streams every output file under <id>/out/ into w as one flat zip,
// suffixing basename collisions (book.epub, book-1.epub, ...) against the set
// of names already written to the archive.
func (s *Store) WriteZip(id string, w io.Writer) (err error) {
	if !ValidID(id) {
		return fmt.Errorf("invalid id %q", id)
	}
	outRoot := filepath.Join(s.root(id), "out")
	zw := zip.NewWriter(w)
	defer func() {
		// zip.Writer.Close() flushes the central directory to w; without this,
		// a failure there would be silently dropped and callers would treat a
		// truncated archive as valid.
		if cerr := zw.Close(); err == nil {
			err = cerr
		}
	}()

	written := map[string]bool{}
	return filepath.WalkDir(outRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() || strings.HasPrefix(d.Name(), ".") {
			return nil
		}
		name := uniqueZipName(d.Name(), written)
		written[name] = true

		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		hw, err := zw.Create(name)
		if err != nil {
			return err
		}
		_, err = io.Copy(hw, f)
		return err
	})
}

// uniqueZipName returns name, suffixed against the set of names already
// written (book.epub -> book-1.epub -> book-2.epub, ...) until it is unique.
// Keying off the set of FINAL chosen names (rather than a per-original-
// basename counter) prevents a suffixed name from silently colliding with a
// different input that genuinely produces that same suffixed basename.
func uniqueZipName(name string, written map[string]bool) string {
	if !written[name] {
		return name
	}
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	// Strip any pre-existing "-N" suffix so repeated collisions increment
	// cleanly (book-1.epub colliding again yields book-2.epub, not
	// book-1-1.epub).
	if i := strings.LastIndex(stem, "-"); i >= 0 {
		if _, err := strconv.Atoi(stem[i+1:]); err == nil {
			stem = stem[:i]
		}
	}
	for n := 1; ; n++ {
		candidate := stem + "-" + strconv.Itoa(n) + ext
		if !written[candidate] {
			return candidate
		}
	}
}

// Sweep deletes job dirs whose mtime is older than the TTL and returns the count.
func (s *Store) Sweep(now time.Time) (removed int) {
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		return 0
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if now.Sub(info.ModTime()) > s.TTL {
			if os.RemoveAll(filepath.Join(s.Dir, e.Name())) == nil {
				removed++
			}
		}
	}
	return removed
}
