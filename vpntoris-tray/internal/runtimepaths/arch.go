package runtimepaths

import "runtime"

func architecture() string {
	return runtime.GOARCH
}
