// Package store handles managed files under a local storage root.
package store

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

var (
	ErrTooLarge    = errors.New("media exceeds byte limit")
	ErrUnsupported = errors.New("unsupported media type")
)

// Local stores files by caller-assigned, root-relative keys.
type Local struct {
	dir string
}

// NewLocal creates the storage directory if necessary.
func NewLocal(dir string) (*Local, error) {
	if dir == "" {
		return nil, errors.New("storage directory is required")
	}
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &Local{dir: dir}, nil
}

// Metadata describes a successfully published file.
type Metadata struct {
	SHA256 string
	Size   int64
	MIME   string
}

// PutOptions controls upload limits and optional application validation.
// Validate sees the staging file positioned at its start, before publication.
type PutOptions struct {
	MaxBytes int64
	Accept   func(mime string) bool
	Validate func(mime string, src io.ReadSeeker) error
}

// Put streams a bounded upload to key. Until validation and publication
// succeed, readers continue to see the previous file, if any.
func (s *Local) Put(key string, src io.Reader, options PutOptions) (Metadata, error) {
	if options.MaxBytes <= 0 {
		return Metadata{}, errors.New("a positive byte limit is required")
	}
	limited := &io.LimitedReader{R: src, N: options.MaxBytes}
	buffered := bufio.NewReader(limited)
	head, err := buffered.Peek(int(min(512, options.MaxBytes)))
	if err != nil && err != io.EOF {
		return Metadata{}, err
	}
	mime := DetectMIME(head)
	if options.Accept != nil && !options.Accept(mime) {
		return Metadata{}, fmt.Errorf("%w: %s", ErrUnsupported, mime)
	}

	digest := sha256.New()
	var n int64
	err = s.write(key, func(f *os.File) error {
		var err error
		n, err = io.Copy(io.MultiWriter(f, digest), buffered)
		if err != nil {
			return err
		}
		if limited.N == 0 {
			var extra [1]byte
			if _, err := io.ReadFull(src, extra[:]); err == nil {
				return ErrTooLarge
			} else if err != io.EOF {
				return err
			}
		}
		if options.Validate != nil {
			if _, err := f.Seek(0, io.SeekStart); err != nil {
				return err
			}
			return options.Validate(mime, f)
		}
		return nil
	})
	if err != nil {
		return Metadata{}, err
	}
	return Metadata{SHA256: hex.EncodeToString(digest.Sum(nil)), Size: n, MIME: mime}, nil
}

// DetectMIME sniffs up to the first 512 bytes, including Ogg and FLAC audio.
func DetectMIME(head []byte) string {
	if bytes.HasPrefix(head, []byte("fLaC")) {
		return "audio/flac"
	}
	mime := http.DetectContentType(head)
	if mime == "application/ogg" {
		return "audio/ogg"
	}
	return mime
}

// Write publishes a generated file atomically after encode succeeds.
func (s *Local) Write(key string, encode func(io.Writer) error) error {
	return s.write(key, func(f *os.File) error { return encode(f) })
}

func (s *Local) write(key string, encode func(*os.File) error) error {
	root, err := s.openRoot(key)
	if err != nil {
		return err
	}
	defer root.Close()

	dir := filepath.Dir(key)
	if err := root.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp := filepath.Join(dir, ".incoming-"+rand.Text())
	f, err := root.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer root.Remove(tmp)
	defer f.Close()
	if err := encode(f); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return root.Rename(tmp, key)
}

// Open opens a stored file; the caller closes it.
func (s *Local) Open(key string) (*os.File, error) {
	root, err := s.openRoot(key)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	return root.Open(key)
}

// RemoveAll removes a key and any descendants without removing the storage root.
func (s *Local) RemoveAll(key string) error {
	root, err := s.openRoot(key)
	if err != nil {
		return err
	}
	defer root.Close()
	return root.RemoveAll(key)
}

func (s *Local) openRoot(key string) (*os.Root, error) {
	if !filepath.IsLocal(key) || filepath.Clean(key) != key || key == "." {
		return nil, errors.New("invalid storage key")
	}
	return os.OpenRoot(s.dir)
}
