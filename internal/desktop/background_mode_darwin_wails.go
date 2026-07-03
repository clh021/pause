//go:build wails && darwin

package desktop

func SupportsBackgroundWindowing() bool {
	return true
}
