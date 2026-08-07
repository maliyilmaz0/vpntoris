//go:build darwin

package nativehelper

func charonKernelPlugins() string {
	return "    kernel-pfkey { load = yes }\n    kernel-pfroute { load = yes }\n"
}
