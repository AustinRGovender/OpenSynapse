package artifacts

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// uploadPartSize bounds the memory one artifact upload buffers.
const uploadPartSize = 16 << 20

// S3 stores artifacts in an S3-compatible bucket (MinIO in the Helm chart).
type S3 struct {
	client *minio.Client
	bucket string
	where  string
}

// S3Config describes an S3-compatible endpoint.
type S3Config struct {
	Endpoint  string // host:port, without scheme
	Bucket    string
	AccessKey string
	SecretKey string
	UseSSL    bool
}

// NewS3 connects to the endpoint and creates the bucket if it does not exist.
// It retries for a while, since MinIO may start alongside the control plane.
func NewS3(ctx context.Context, cfg S3Config) (*S3, error) {
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("object storage client: %w", err)
	}
	scheme := "http"
	if cfg.UseSSL {
		scheme = "https"
	}
	s := &S3{client: client, bucket: cfg.Bucket, where: fmt.Sprintf("s3 bucket %q at %s://%s", cfg.Bucket, scheme, cfg.Endpoint)}

	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	for {
		exists, err := client.BucketExists(ctx, cfg.Bucket)
		if err == nil && !exists {
			err = client.MakeBucket(ctx, cfg.Bucket, minio.MakeBucketOptions{})
		}
		if err == nil {
			return s, nil
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("object storage %s: %w", s.where, err)
		case <-time.After(2 * time.Second):
		}
	}
}

// s3Writer streams into a multipart upload; Close waits for it to finish.
type s3Writer struct {
	pw   *io.PipeWriter
	done chan error
}

func (w *s3Writer) Write(p []byte) (int, error) { return w.pw.Write(p) }

func (w *s3Writer) Close() error {
	w.pw.Close()
	return <-w.done
}

func (s *S3) Create(ctx context.Context, key string) (io.WriteCloser, error) {
	pr, pw := io.Pipe()
	w := &s3Writer{pw: pw, done: make(chan error, 1)}
	go func() {
		// With an unknown length the client would size parts for a 5 TiB
		// object (~537 MiB, buffered in memory); 16 MiB parts allow archives
		// up to ~160 GiB in a fraction of the memory.
		_, err := s.client.PutObject(context.WithoutCancel(ctx), s.bucket, key, pr, -1, minio.PutObjectOptions{ContentType: "application/gzip", PartSize: uploadPartSize})
		pr.CloseWithError(err)
		w.done <- err
	}()
	return w, nil
}

func (s *S3) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	if _, err := s.client.StatObject(ctx, s.bucket, key, minio.StatObjectOptions{}); err != nil {
		if minio.ToErrorResponse(err).Code == minio.NoSuchKey {
			return nil, fmt.Errorf("%s: %w", key, fs.ErrNotExist)
		}
		return nil, err
	}
	return s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
}

func (s *S3) DeletePrefix(ctx context.Context, prefix string) error {
	objects := s.client.ListObjects(ctx, s.bucket, minio.ListObjectsOptions{Prefix: prefix, Recursive: true})
	for res := range s.client.RemoveObjects(ctx, s.bucket, objects, minio.RemoveObjectsOptions{}) {
		if res.Err != nil {
			return res.Err
		}
	}
	return nil
}

func (s *S3) Size(ctx context.Context, prefix string) (int64, error) {
	var total int64
	for obj := range s.client.ListObjects(ctx, s.bucket, minio.ListObjectsOptions{Prefix: strings.TrimPrefix(prefix, "/"), Recursive: true}) {
		if obj.Err != nil {
			return total, obj.Err
		}
		total += obj.Size
	}
	return total, nil
}

func (s *S3) Location() string { return s.where }
