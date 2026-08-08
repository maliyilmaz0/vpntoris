//go:build linux

package helperipc

import (
	"fmt"
	"net"

	"golang.org/x/sys/unix"
)

// peerCredentialsUID returns the uid of the process on the other end of a
// Unix-domain socket via SO_PEERCRED.
func peerCredentialsUID(connection net.Conn) (uint32, error) {
	unixConn, ok := connection.(*net.UnixConn)
	if !ok {
		return 0, fmt.Errorf("not a unix connection")
	}
	raw, err := unixConn.SyscallConn()
	if err != nil {
		return 0, err
	}
	var cred *unix.Ucred
	var credErr error
	if err := raw.Control(func(fd uintptr) {
		cred, credErr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	}); err != nil {
		return 0, err
	}
	if credErr != nil {
		return 0, credErr
	}
	return cred.Uid, nil
}
