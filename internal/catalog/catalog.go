// Package catalog lists themes and plugins that can be installed from the GUI.
// Entries come from the catalog bundled in the binary and from any number of
// remote index.json URLs the user adds.
package catalog

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"meofishes/internal/pkgfs"
)

// Entry is one installable package.
type Entry struct {
	Kind        string            `json:"kind"` // theme | plugin
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Version     string            `json:"version,omitempty"`
	Author      string            `json:"author,omitempty"`
	Description string            `json:"description,omitempty"`
	Types       []string          `json:"types,omitempty"`   // plugins
	Runtime     string            `json:"runtime,omitempty"` // plugins
	Preview     map[string]string `json:"preview,omitempty"` // themes: color swatches
	URL         string            `json:"url,omitempty"`     // .zip to download
	Bundled     string            `json:"bundled,omitempty"` // path inside the app's embedded files
	Default     bool              `json:"default,omitempty"` // installed on first run
	Catalog     string            `json:"-"`                 // where the entry came from
}

// Index is the catalog file format.
type Index struct {
	Name    string  `json:"name"`
	Entries []Entry `json:"entries"`
}

// Catalog aggregates the bundled index and remote ones.
type Catalog struct {
	Bundled fs.FS // embedded assets (catalog.json + folders)
	mu      sync.Mutex
	remote  map[string]*Index
}

func (c *Catalog) bundledIndex() *Index {
	data, err := fs.ReadFile(c.Bundled, "catalog.json")
	if err != nil {
		return &Index{}
	}
	var idx Index
	_ = json.Unmarshal(data, &idx)
	for i := range idx.Entries {
		idx.Entries[i].Catalog = "meofishes"
	}
	return &idx
}

// Fetch downloads a remote index (cached for this session).
func (c *Catalog) Fetch(url string, ua string) (*Index, error) {
	c.mu.Lock()
	if c.remote == nil {
		c.remote = map[string]*Index{}
	}
	if idx, ok := c.remote[url]; ok {
		c.mu.Unlock()
		return idx, nil
	}
	c.mu.Unlock()
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", ua)
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("catalog %s: http %d", url, resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	var idx Index
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, fmt.Errorf("catalog %s: %w", url, err)
	}
	name := idx.Name
	if name == "" {
		name = url
	}
	for i := range idx.Entries {
		idx.Entries[i].Catalog = name
		idx.Entries[i].Bundled = "" // remote catalogs can only point at URLs
		u := idx.Entries[i].URL
		if u != "" && !strings.HasPrefix(u, "http") { // relative to the index
			idx.Entries[i].URL = url[:strings.LastIndex(url, "/")+1] + u
		}
	}
	c.mu.Lock()
	c.remote[url] = &idx
	c.mu.Unlock()
	return &idx, nil
}

// Refresh forgets remote indexes so they are downloaded again.
func (c *Catalog) Refresh() { c.mu.Lock(); c.remote = nil; c.mu.Unlock() }

// List returns every entry of a kind. Errors of remote catalogs are returned
// alongside the entries that did load.
func (c *Catalog) List(kind string, remotes []string, ua string) ([]Entry, []error) {
	var out []Entry
	var errs []error
	add := func(idx *Index) {
		for _, e := range idx.Entries {
			if e.Kind == kind {
				out = append(out, e)
			}
		}
	}
	add(c.bundledIndex())
	for _, u := range remotes {
		if strings.TrimSpace(u) == "" {
			continue
		}
		idx, err := c.Fetch(u, ua)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		add(idx)
	}
	return out, errs
}

// Defaults returns bundled entries flagged "default".
func (c *Catalog) Defaults() []Entry {
	var out []Entry
	for _, e := range c.bundledIndex().Entries {
		if e.Default {
			out = append(out, e)
		}
	}
	return out
}

// Materialize returns a local path or URL the installers understand.
// For bundled entries the files are copied to a temp folder.
func (c *Catalog) Materialize(e Entry) (src string, cleanup func(), err error) {
	if e.Bundled != "" {
		dir, err := os.MkdirTemp("", "meofishes-bundled-*")
		if err != nil {
			return "", func() {}, err
		}
		sub, err := fs.Sub(c.Bundled, e.Bundled)
		if err == nil {
			err = pkgfs.CopyFS(sub, ".", dir)
		}
		if err != nil {
			os.RemoveAll(dir)
			return "", func() {}, err
		}
		return dir, func() { os.RemoveAll(dir) }, nil
	}
	if e.URL == "" {
		return "", func() {}, fmt.Errorf("entry %s has no url", e.ID)
	}
	return e.URL, func() {}, nil
}
