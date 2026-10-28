package artifacts

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// exercise runs the same checks against any Store.
func exercise(t *testing.T, s Store) {
	t.Helper()
	ctx := context.Background()
	key := RunSamplesKey("run-1")

	if _, err := s.Open(ctx, key); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Open(missing) = %v, want fs.ErrNotExist", err)
	}

	// Larger than one multipart part, written in small chunks of unknown total size.
	payload := bytes.Repeat([]byte("0123456789abcdef"), 1<<20) // 16 MiB
	w, err := s.Create(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < len(payload); i += 64 * 1024 {
		if _, err := w.Write(payload[i : i+64*1024]); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	w2, _ := s.Create(ctx, "runs/run-1/other.txt")
	w2.Write([]byte("x"))
	w2.Close()
	w3, _ := s.Create(ctx, RunSamplesKey("run-2"))
	w3.Write([]byte("keep"))
	w3.Close()

	r, err := s.Open(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(r)
	r.Close()
	if !bytes.Equal(got, payload) {
		t.Fatalf("read back %d bytes, want %d", len(got), len(payload))
	}

	if n, err := s.Size(ctx, RunPrefix("run-1")); err != nil || n != int64(len(payload)+1) {
		t.Fatalf("Size(run-1) = %d, %v; want %d", n, err, len(payload)+1)
	}
	if err := s.DeletePrefix(ctx, RunPrefix("run-1")); err != nil {
		t.Fatal(err)
	}
	if n, _ := s.Size(ctx, RunPrefix("run-1")); n != 0 {
		t.Fatalf("Size after delete = %d", n)
	}
	if n, _ := s.Size(ctx, "runs/"); n != 4 {
		t.Fatalf("other runs' artifacts were affected: %d bytes left, want 4", n)
	}
}

func TestDirStore(t *testing.T) {
	exercise(t, NewDir(t.TempDir()))
}

// Keys are cleaned as absolute paths, so ".." cannot climb out of the root.
func TestDirStoreContainsEscapingKeys(t *testing.T) {
	root := t.TempDir()
	d := NewDir(filepath.Join(root, "store"))
	w, err := d.Create(context.Background(), "../../outside.txt")
	if err != nil {
		t.Fatal(err)
	}
	w.Close()
	if _, err := os.Stat(filepath.Join(root, "outside.txt")); err == nil {
		t.Fatal("artifact was written outside the store's root")
	}
	if _, err := os.Stat(filepath.Join(root, "store", "outside.txt")); err != nil {
		t.Fatalf("artifact not inside the root: %v", err)
	}
}

// TestS3Store runs against a real S3-compatible server when
// OPENSYNAPSE_TEST_S3 is set to endpoint,access-key,secret-key.
func TestS3Store(t *testing.T) {
	cfg := os.Getenv("OPENSYNAPSE_TEST_S3")
	if cfg == "" {
		t.Skip("set OPENSYNAPSE_TEST_S3=host:port,access,secret to test against object storage")
	}
	parts := strings.Split(cfg, ",")
	s, err := NewS3(context.Background(), S3Config{Endpoint: parts[0], AccessKey: parts[1], SecretKey: parts[2], Bucket: "opensynapse-test"})
	if err != nil {
		t.Fatal(err)
	}
	exercise(t, s)
}
