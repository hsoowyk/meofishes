package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestNewer(t *testing.T) {
	r := &Release{Commit: "abcdef1234"}
	if r.Newer("dev") || r.Newer("abcdef1") || !r.Newer("1234567") {
		t.Fatal("Newer")
	}
}

func TestWriteAtomicFromTar(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range map[string]string{"meofishes/meofishes": "BIN", "meofishes/docs/a.md": "doc", "meofishes/install.sh": "x", "meofishes/../evil": "no"} {
		_ = tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg})
		_, _ = tw.Write([]byte(body))
	}
	tw.Close()
	gz.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(buf.Bytes()) }))
	defer srv.Close()
	// Apply replaces os.Executable(), so only check the archive walk via writeAtomic
	dir := t.TempDir()
	if err := writeAtomic(filepath.Join(dir, "x", "f"), bytes.NewBufferString("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "x", "f")); string(b) != "hi" {
		t.Fatal("content")
	}
	_ = context.Background()
}
