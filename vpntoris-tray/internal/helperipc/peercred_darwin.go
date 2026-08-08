//go:build darwin

package helperipc

import (
	"errors"
	"net"
)

func peerCredentialsUID(connection net.Conn) (uint32, error) {
	return 0, errors.New("peer credentials not supported on darwin")
}
