//go:build windows

package nativehelper

import "os"

func chownUser(*os.File, int) error {
	return nil
}
