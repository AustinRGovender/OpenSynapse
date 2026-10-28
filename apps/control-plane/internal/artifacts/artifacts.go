// Package artifacts stores per-run files that do not belong in the database,
// such as the gzip archive of raw k6 samples. A local install keeps them in
// the data directory; a cluster install keeps them in S3-compatible object
// storage (MinIO), so the control plane itself can stay stateless.
package artifacts

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Store holds artifacts by slash-separated key, e.g. "runs/<id>/samples.jsonl.gz".
type Store interface {
	// Create starts writing an artifact; it is stored when the writer is closed.
	Create(ctx context.Context, key string) (io.WriteCloser, error)
	// Open reads an artifact. Missing artifacts return an error matching fs.ErrNotExist.
	Open(ctx context.Context, key string) (io.ReadCloser, error)
	// DeletePrefix removes every artifact whose key starts with prefix.
	DeletePrefix(ctx context.Context, prefix string) error
	// Size totals the bytes stored under prefix.
	Size(ctx context.Context, prefix string) (int64, error)
	// Location describes where artifacts live, for the storage settings page.
	Location() string
}

// RunSamplesKey is where a run's raw sample archive is stored.
func RunSamplesKey(runID string) string { return "runs/" + runID + "/samples.jsonl.gz" }

// RunPrefix covers every artifact of a run.
func RunPrefix(runID string) string { return "runs/" + runID + "/" }

// Dir stores artifacts as files under a directory.
type Dir struct{ Root string }

// NewDir returns a Store rooted at dir.
func NewDir(dir string) *Dir { return &Dir{Root: dir} }

func (d *Dir) path(key string) (string, error) {
	clean := path.Clean("/" + key)[1:]
	if clean == "" || strings.HasPrefix(clean, "..") {
		return "", errors.New("invalid artifact key")
	}
	return filepath.Join(d.Root, filepath.FromSlash(clean)), nil
}

func (d *Dir) Create(_ context.Context, key string) (io.WriteCloser, error) {
	p, err := d.path(key)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return nil, err
	}
	return os.Create(p)
}

func (d *Dir) Open(_ context.Context, key string) (io.ReadCloser, error) {
	p, err := d.path(key)
	if err != nil {
		return nil, err
	}
	return os.Open(p)
}

func (d *Dir) DeletePrefix(_ context.Context, prefix string) error {
	p, err := d.path(strings.TrimSuffix(prefix, "/"))
	if err != nil {
		return err
	}
	return os.RemoveAll(p)
}

func (d *Dir) Size(_ context.Context, prefix string) (int64, error) {
	p, err := d.path(strings.TrimSuffix(prefix, "/"))
	if err != nil {
		return 0, err
	}
	var total int64
	err = filepath.WalkDir(p, func(_ string, e fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if info, err := e.Info(); err == nil && !e.IsDir() {
			total += info.Size()
		}
		return nil
	})
	return total, err
}

func (d *Dir) Location() string { return d.Root }
