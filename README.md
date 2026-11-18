# gomedia

Go packages for managed file bytes and image thumbnails. Applications own
storage keys, HTTP endpoints, authentication, database metadata, and the
lifecycle of their references to stored files.

- `store`: local disk, root-scoped filesystem operations, byte limits, SHA-256,
  MIME sniffing, and atomic publication after validation.
- `imaging`: pixel-budgeted image probing and decoding, plus JPEG/PNG
  thumbnails without embedded source metadata.

Use the Go module directly or pin it in a consuming repo at `pkg/gomedia`
with a local Go module replacement:

```go
replace github.com/cyphix/gomedia => ./pkg/gomedia
```

Initialize the submodule in fresh checkouts and include it in container builds
before downloading Go dependencies. When updating the library, merge its
change and pin the resulting commit in the consuming repo.

Run `go test ./...` and `go vet ./...` from the module root.
