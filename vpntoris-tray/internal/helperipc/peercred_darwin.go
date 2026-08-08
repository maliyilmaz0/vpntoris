//go:build darwin

package helperipc

import (
	"errors"
	"net"
)

// peerCredentialsUID is only used on the Linux multi-user path (uid == 0).
// On macOS the socket is chowned to the console user (uid > 0), so this is
// never called; return an error to fail closed if that ever changes.
func peerCredentialsUID(connection net.Conn) (uint32, error) {
	return 0, errors.New("peer credentials not supported on darwin")
}
