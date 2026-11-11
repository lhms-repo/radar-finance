//go:build !embedweb

// Package web exposes the built React frontend embedded in the binary.
package web

import "io/fs"

// Assets returns nil in development builds, where Vite serves the frontend.
func Assets() fs.FS {
	return nil
}
