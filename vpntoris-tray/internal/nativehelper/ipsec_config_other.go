//go:build !darwin && !linux

package nativehelper

func charonKernelPlugins() string {
	return ""
}
