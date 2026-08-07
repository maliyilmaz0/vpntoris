//go:build linux

package nativehelper

func charonKernelPlugins() string {
	return "    kernel-netlink { load = yes }\n"
}
