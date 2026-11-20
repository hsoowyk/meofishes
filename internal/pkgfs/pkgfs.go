// Package pkgfs fetches and unpacks plugin and theme packages (folders, .zip
// files or URLs to .zip files).
package pkgfs

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// MaxPackage is the biggest package we download or unpack (64 MiB).
const MaxPackage = 64 << 20

// Fetch makes src available as a local folder. src may be a folder, a .zip
// file or an http(s) URL to a .zip. cleanup removes temporary files.
func Fetch(src string) (dir string, cleanup func(), err error) {
	noop := func() {}
	if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
		tmp, err := os.CreateTemp("", "meofishes-pkg-*.zip")
		if err != nil {
			return "", noop, err
		}
		defer tmp.Close()
		c := &http.Client{Timeout: 2 * time.Minute}
		resp, err := c.Get(src)
		if err != nil {
			os.Remove(tmp.Name())
			return "", noop, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			os.Remove(tmp.Name())
			return "", noop, fmt.Errorf("download: http %d", resp.StatusCode)
		}
		if _, err := io.Copy(tmp, io.LimitReader(resp.Body, MaxPackage+1)); err != nil {
			os.Remove(tmp.Name())
			return "", noop, err
		}
		d, c2, err := unzipTemp(tmp.Name())
		return d, func() { c2(); os.Remove(tmp.Name()) }, err
	}
	st, err := os.Stat(src)
	if err != nil {
		return "", noop, err
	}
	if st.IsDir() {
		return src, noop, nil
	}
	return unzipTemp(src)
}

func unzipTemp(path string) (string, func(), error) {
	dir, err := os.MkdirTemp("", "meofishes-unpack-*")
	if err != nil {
		return "", func() {}, err
	}
	cleanup := func() { os.RemoveAll(dir) }
	if err := Unzip(path, dir); err != nil {
		cleanup()
		return "", func() {}, err
	}
	return dir, cleanup, nil
}

// Unzip extracts a zip, refusing paths that escape dst.
func Unzip(path, dst string) error {
	r, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	defer r.Close()
	var total uint64
	for _, f := range r.File {
		total += f.UncompressedSize64
		if total > MaxPackage {
			return errors.New("package is too large")
		}
		name := filepath.Clean(f.Name)
		if filepath.IsAbs(name) || strings.HasPrefix(name, ".."+string(os.PathSeparator)) || name == ".." {
			return fmt.Errorf("unsafe path in zip: %s", f.Name)
		}
		target := filepath.Join(dst, name)
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if f.Mode()&fs.ModeSymlink != 0 {
			continue // never follow links from packages
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		mode := os.FileMode(0o644)
		if f.Mode()&0o111 != 0 {
			mode = 0o755
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
		if err != nil {
			rc.Close()
			return err
		}
		_, err = io.Copy(out, io.LimitReader(rc, MaxPackage))
		rc.Close()
		out.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// FindRoot returns the folder holding marker (dir itself or a single subfolder).
func FindRoot(dir, marker string) (string, error) {
	if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
		return dir, nil
	}
	entries, _ := os.ReadDir(dir)
	var subs []string
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") && e.Name() != "__MACOSX" {
			subs = append(subs, e.Name())
		}
	}
	if len(subs) == 1 {
		if _, err := os.Stat(filepath.Join(dir, subs[0], marker)); err == nil {
			return filepath.Join(dir, subs[0]), nil
		}
	}
	return "", fmt.Errorf("no %s found in the package", marker)
}

// ReplaceDir copies src over dst (dst is removed first).
func ReplaceDir(src, dst string) error {
	if abs1, _ := filepath.Abs(src); abs1 != "" {
		if abs2, _ := filepath.Abs(dst); abs1 == abs2 {
			return nil
		}
	}
	tmp := dst + ".new"
	os.RemoveAll(tmp)
	if err := CopyFS(os.DirFS(src), ".", tmp); err != nil {
		os.RemoveAll(tmp)
		return err
	}
	os.RemoveAll(dst)
	return os.Rename(tmp, dst)
}

// CopyFS copies root of fsys into dst.
func CopyFS(fsys fs.FS, root, dst string) error {
	return fs.WalkDir(fsys, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		data, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		mode := os.FileMode(0o644)
		if info, err := d.Info(); err == nil && info.Mode()&0o111 != 0 {
			mode = 0o755
		}
		return os.WriteFile(target, data, mode)
	})
}
