//go:build windows && !nircmdembed

package remoteserver

import "fmt"

// getEmbeddedNircmdPath returns an error because the build was not configured
// with the nircmdembed tag (nircmd.exe was not embedded). Callers should
// fall back to looking up nircmd.exe in PATH or use GDI capture.
func getEmbeddedNircmdPath() (string, func(), error) {
	return "", nil, fmt.Errorf("nircmd.exe not embedded (build without nircmdembed tag)")
}
