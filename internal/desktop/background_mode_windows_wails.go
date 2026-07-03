//go:build wails && windows

package desktop

func SupportsBackgroundWindowing() bool {
	return true
}
