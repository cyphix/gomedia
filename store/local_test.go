package store_test

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/cyphix/gomedia/store"
)

func TestPutPublishesValidatedFile(t *testing.T) {
	s, err := store.NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const payload = "fLaC example data"
	metadata, err := s.Put("items/1/original", strings.NewReader(payload), store.PutOptions{
		MaxBytes: int64(len(payload)),
		Accept:   func(mime string) bool { return mime == "audio/flac" },
		Validate: func(_ string, src io.ReadSeeker) error {
			got, err := io.ReadAll(src)
			if err != nil || string(got) != payload {
				return fmt.Errorf("staged contents = %q, %v", got, err)
			}
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if metadata.MIME != "audio/flac" || metadata.Size != int64(len(payload)) ||
		metadata.SHA256 != fmt.Sprintf("%x", sha256.Sum256([]byte(payload))) {
		t.Fatalf("unexpected metadata: %+v", metadata)
	}
	f, err := s.Open("items/1/original")
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(f)
	f.Close()
	if err != nil || string(got) != payload {
		t.Fatalf("stored contents = %q, %v", got, err)
	}
	if err := s.RemoveAll("items/1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Open("items/1/original"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("open after delete: %v", err)
	}
}

func TestPutFailuresPreserveOriginal(t *testing.T) {
	dir := t.TempDir()
	s, err := store.NewLocal(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Put("original", strings.NewReader("old"), store.PutOptions{MaxBytes: 16}); err != nil {
		t.Fatal(err)
	}
	invalid := errors.New("invalid contents")
	broken := errors.New("reader failed")
	tests := []struct {
		name string
		src  io.Reader
		opts store.PutOptions
		err  error
	}{
		{"size", strings.NewReader("too big"), store.PutOptions{MaxBytes: 4}, store.ErrTooLarge},
		{"type", strings.NewReader("bad type"), store.PutOptions{
			MaxBytes: 16, Accept: func(string) bool { return false },
		}, store.ErrUnsupported},
		{"validation", strings.NewReader("invalid"), store.PutOptions{
			MaxBytes: 16, Validate: func(string, io.ReadSeeker) error { return invalid },
		}, invalid},
		{"read", io.MultiReader(strings.NewReader(strings.Repeat("x", 600)), iotest.ErrReader(broken)),
			store.PutOptions{MaxBytes: 1024}, broken},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := s.Put("original", tc.src, tc.opts); !errors.Is(err, tc.err) {
				t.Fatalf("Put: %v, want %v", err, tc.err)
			}
			got, err := os.ReadFile(filepath.Join(dir, "original"))
			if err != nil || string(got) != "old" {
				t.Fatalf("original after failure = %q, %v", got, err)
			}
			files, err := os.ReadDir(dir)
			if err != nil || len(files) != 1 {
				t.Fatalf("storage entries after failure = %v, %v", files, err)
			}
		})
	}
}

func TestRootConfinement(t *testing.T) {
	dir, outside := t.TempDir(), t.TempDir()
	s, err := store.NewLocal(dir)
	if err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(outside, "original")
	if err := os.WriteFile(marker, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "link")); err != nil {
		t.Skipf("cannot create test symlink: %v", err)
	}
	for _, key := range []string{".", "..", marker, filepath.Join("link", "original")} {
		if _, err := s.Open(key); err == nil {
			t.Errorf("opened %q", key)
		}
		if _, err := s.Put(key, strings.NewReader("new"), store.PutOptions{MaxBytes: 16}); err == nil {
			t.Errorf("wrote %q", key)
		}
		if err := s.RemoveAll(key); err == nil {
			t.Errorf("removed %q", key)
		}
	}
	got, err := os.ReadFile(marker)
	if err != nil || string(got) != "outside" {
		t.Fatalf("outside contents = %q, %v", got, err)
	}
}
