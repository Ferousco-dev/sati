package files

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type Storage interface {
	Put(ctx context.Context, key string, r io.Reader) error
	Delete(ctx context.Context, key string) error
	URL(key string) string
}

const LocalURLPrefix = "/api/v1/files/"

type LocalStorage struct {
	dir     string
	baseURL string
}

// NewLocalStorage serves files under baseURL, which can point at a CDN
// domain. An empty baseURL uses the API's own file route.
func NewLocalStorage(dir, baseURL string) (*LocalStorage, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	if baseURL == "" {
		baseURL = LocalURLPrefix
	}
	return &LocalStorage{dir: dir, baseURL: strings.TrimRight(baseURL, "/") + "/"}, nil
}

func (s *LocalStorage) path(key string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(key))
	if clean == "." || filepath.IsAbs(clean) || strings.HasPrefix(clean, "..") {
		return "", errors.New("invalid storage key")
	}
	return filepath.Join(s.dir, clean), nil
}

func (s *LocalStorage) Put(_ context.Context, key string, r io.Reader) error {
	path, err := s.path(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		os.Remove(path)
		return err
	}
	return f.Close()
}

func (s *LocalStorage) Delete(_ context.Context, key string) error {
	path, err := s.path(key)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (s *LocalStorage) URL(key string) string { return s.baseURL + key }

type cacheOnSuccess struct {
	http.ResponseWriter
	wrote bool
}

func (c *cacheOnSuccess) WriteHeader(code int) {
	if !c.wrote {
		c.wrote = true
		if code == http.StatusOK || code == http.StatusNotModified || code == http.StatusPartialContent {
			c.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
	}
	c.ResponseWriter.WriteHeader(code)
}

func (c *cacheOnSuccess) Write(p []byte) (int, error) {
	if !c.wrote {
		c.WriteHeader(http.StatusOK)
	}
	return c.ResponseWriter.Write(p)
}

// Stored keys are random and never reused, so a file's URL never changes
// content and can be cached forever. Errors are not cached.
func (s *LocalStorage) Handler() http.Handler {
	files := http.StripPrefix(LocalURLPrefix, http.FileServer(http.Dir(s.dir)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		files.ServeHTTP(&cacheOnSuccess{ResponseWriter: w}, r)
	})
}
