//go:build windows && nircmdembed

package remoteserver

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed nircmd/nircmd.exe
var nircmdEmbedded []byte

// getEmbeddedNircmdPath extracts the embedded nircmd.exe to a temporary
// directory and returns its path along with a cleanup function.
// The caller must invoke cleanup after the capture completes.
func getEmbeddedNircmdPath() (string, func(), error) {
	if len(nircmdEmbedded) == 0 {
		return "", nil, fmt.Errorf("embedded nircmd.exe is empty")
	}
	tmpDir, err := os.MkdirTemp("", "pause-nircmd-*")
	if err != nil {
		return "", nil, fmt.Errorf("create temp dir: %w", err)
	}
	exePath := filepath.Join(tmpDir, "nircmd.exe")
	if err := os.WriteFile(exePath, nircmdEmbedded, 0o700); err != nil {
		os.RemoveAll(tmpDir)
		return "", nil, fmt.Errorf("write nircmd.exe: %w", err)
	}
	return exePath, func() { os.RemoveAll(tmpDir) }, nil
}
