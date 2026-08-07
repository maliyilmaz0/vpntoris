//go:build windows

package nativehelper

func processAlive(int) bool {
	return false
}
