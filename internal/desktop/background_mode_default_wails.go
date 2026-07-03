//go:build wails && !darwin && !windows

package desktop

func SupportsBackgroundWindowing() bool {
	return false
}
