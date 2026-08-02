package jobs

import (
	"archive/zip"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// WriteZip streams every output file under <id>/out/ into w as one flat zip,
// suffixing basename collisions (book.epub, book-1.epub, ...).
func (s *Store) WriteZip(id string, w io.Writer) error {
	outRoot := filepath.Join(s.root(id), "out")
	zw := zip.NewWriter(w)
	defer zw.Close()

	seen := map[string]int{}
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
		name := d.Name()
		if n := seen[d.Name()]; n > 0 {
			ext := filepath.Ext(name)
			name = strings.TrimSuffix(name, ext) + "-" + strconv.Itoa(n) + ext
		}
		seen[d.Name()]++

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
