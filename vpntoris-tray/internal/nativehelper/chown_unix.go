//go:build unix

package nativehelper

import "os"

func chownUser(file *os.File, userID int) error {
	if userID < 0 {
		return nil
	}
	return file.Chown(userID, -1)
}
