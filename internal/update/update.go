// Package update installs new builds published by the CI (the "nightly"
// GitHub release of the project) and uninstalls the app.
package update

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Asset is the file the CI publishes for this system.
var Asset = assetName(runtime.GOOS, runtime.GOARCH)

func assetName(goos, arch string) string {
	if arch == "amd64" {
		arch = "x86_64"
	}
	switch goos {
	case "darwin":
		return "meofishes-macos-" + arch + ".tar.gz"
	case "windows":
		return "meofishes-windows-" + arch + ".zip"
	}
	return "meofishes-" + goos + "-" + arch + ".tar.gz"
}

// Release is the latest build.
type Release struct {
	Commit    string    `json:"commit"`
	URL       string    `json:"url"`
	Published time.Time `json:"published"`
	Notes     string    `json:"notes"`
}

// Short is the first 7 characters of the commit.
func (r *Release) Short() string {
	if len(r.Commit) > 7 {
		return r.Commit[:7]
	}
	return r.Commit
}

// Latest asks GitHub for the nightly release of repo ("owner/name").
func Latest(ctx context.Context, repo, ua string) (*Release, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.github.com/repos/"+repo+"/releases/tags/nightly", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", ua)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return nil, errors.New("no build published yet")
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("github: http %d", resp.StatusCode)
	}
	var r struct {
		Target    string    `json:"target_commitish"`
		Body      string    `json:"body"`
		Published time.Time `json:"published_at"`
		Assets    []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, err
	}
	rel := &Release{Commit: r.Target, Published: r.Published, Notes: r.Body}
	for _, a := range r.Assets {
		if a.Name == Asset {
			rel.URL = a.URL
		}
	}
	if rel.URL == "" {
		return nil, errors.New("the release has no " + Asset)
	}
	return rel, nil
}

// Newer reports whether r is a different build than current (a commit).
// Development builds ("dev") are never updated automatically.
func (r *Release) Newer(current string) bool {
	if current == "" || current == "dev" || r.Commit == "" {
		return false
	}
	return !strings.HasPrefix(r.Commit, current) && !strings.HasPrefix(current, r.Commit)
}

// Apply downloads the release and replaces the running executable (and the
// docs and icon next to it). The new version runs after a restart.
func Apply(ctx context.Context, r *Release, ua string, progress func(done, total int64)) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	dir := filepath.Dir(exe)
	if err := writable(dir); err != nil {
		return fmt.Errorf("%s is not writable (installed for all users? run: sudo meofishes update)", dir)
	}
	req, err := http.NewRequestWithContext(ctx, "GET", r.URL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", ua)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("download: http %d", resp.StatusCode)
	}
	body := io.Reader(resp.Body)
	if progress != nil {
		body = &counter{r: resp.Body, total: resp.ContentLength, f: progress}
	}
	if strings.HasSuffix(r.URL, ".zip") {
		return applyZip(body, exe, dir)
	}
	gz, err := gzip.NewReader(body)
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	gotBinary := false
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		// entries are meofishes/<path>
		name := filepath.ToSlash(filepath.Clean(h.Name))
		rel, ok := strings.CutPrefix(name, "meofishes/")
		if !ok || rel == "" || strings.Contains(rel, "..") || h.Typeflag != tar.TypeReg {
			continue
		}
		var dst string
		mode := os.FileMode(0o644)
		switch {
		case rel == "meofishes":
			dst, mode = exe, 0o755
		case strings.HasPrefix(rel, "docs/"), strings.HasPrefix(rel, "packaging/"):
			dst = filepath.Join(dir, rel)
		default:
			continue // install.sh is not needed anymore
		}
		if err := writeAtomic(dst, tr, mode); err != nil {
			return err
		}
		if rel == "meofishes" {
			gotBinary = true
		}
	}
	if !gotBinary {
		return errors.New("the archive has no meofishes binary")
	}
	return nil
}

// applyZip installs a Windows build (meofishes/meofishes.exe + docs + the
// Qt files next to it).
func applyZip(body io.Reader, exe, dir string) error {
	tmp, err := os.CreateTemp("", "meofishes-update-*.zip")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := io.Copy(tmp, body); err != nil {
		tmp.Close()
		return err
	}
	tmp.Close()
	zr, err := zip.OpenReader(tmp.Name())
	if err != nil {
		return err
	}
	defer zr.Close()
	got := false
	for _, f := range zr.File {
		name := filepath.ToSlash(filepath.Clean(f.Name))
		rel, ok := strings.CutPrefix(name, "meofishes/")
		if !ok || rel == "" || strings.Contains(rel, "..") || f.FileInfo().IsDir() {
			continue
		}
		dst := filepath.Join(dir, filepath.FromSlash(rel))
		if strings.EqualFold(rel, "meofishes.exe") {
			dst = exe
			got = true
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		err = writeAtomic(dst, rc, 0o755)
		rc.Close()
		if err != nil {
			return err
		}
	}
	if !got {
		return errors.New("the archive has no meofishes.exe")
	}
	return nil
}

// writeAtomic writes to dst.new and renames it over dst (a running
// executable can be replaced this way; on Windows it is moved aside first).
func writeAtomic(dst string, r io.Reader, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	tmp := dst + ".new"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		_ = os.Remove(dst + ".old")
		_ = os.Rename(dst, dst+".old") // a running .exe can be renamed, not replaced
	}
	return os.Rename(tmp, dst)
}

func writable(dir string) error {
	f, err := os.CreateTemp(dir, ".meofishes-write-test-*")
	if err != nil {
		return err
	}
	name := f.Name()
	f.Close()
	return os.Remove(name)
}

type counter struct {
	r     io.Reader
	done  int64
	total int64
	f     func(done, total int64)
}

func (c *counter) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.done += int64(n)
	c.f(c.done, c.total)
	return n, err
}

// ---------------------------------------------------------------- install info

// Install is written by install.sh in ~/.config/meofishes/install.json.
type Install struct {
	Mode    string `json:"mode"` // user | system
	Prefix  string `json:"prefix"`
	Link    string `json:"link"`
	Desktop string `json:"desktop"`
	Icon    string `json:"icon"`
}

// ReadInstall loads the install info.
func ReadInstall(configDir string) (*Install, error) {
	data, err := os.ReadFile(filepath.Join(configDir, "install.json"))
	if err != nil {
		return nil, errors.New("no install info (meofishes was not installed with install.sh)")
	}
	var in Install
	return &in, json.Unmarshal(data, &in)
}

// Uninstall removes the installed files. purge also removes settings,
// library, plugins, themes, downloads index and caches (not downloaded songs).
func Uninstall(configDir, dataDir, cacheDir string, purge bool) ([]string, error) {
	in, err := ReadInstall(configDir)
	if err != nil {
		return nil, err
	}
	if in.Prefix == "" || in.Prefix == "/" || !strings.Contains(in.Prefix, "meofishes") {
		return nil, fmt.Errorf("refusing to remove %q", in.Prefix)
	}
	var removed []string
	for _, p := range []string{in.Link, in.Desktop, in.Icon, in.Prefix} {
		if p == "" {
			continue
		}
		if err := os.RemoveAll(p); err != nil {
			return removed, fmt.Errorf("%v (installed for all users? run: sudo meofishes uninstall)", err)
		}
		removed = append(removed, p)
	}
	_ = os.Remove(filepath.Join(configDir, "install.json"))
	if purge {
		for _, d := range []string{configDir, dataDir, cacheDir} {
			if strings.Contains(d, "meofishes") {
				_ = os.RemoveAll(d)
				removed = append(removed, d)
			}
		}
	}
	return removed, nil
}
