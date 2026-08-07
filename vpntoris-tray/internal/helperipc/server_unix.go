//go:build unix

package helperipc

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"

	"vpntoris-tray/internal/fortihelper"
	"vpntoris-tray/internal/nativehelper"
	"vpntoris-tray/internal/runtimepaths"
)

// ServeUnix starts a Unix-domain control socket for the helper service.
func ServeUnix(service *nativehelper.Service, paths runtimepaths.Paths, uid int) error {
	socketPath := paths.HelperSocket
	if err := os.MkdirAll(filepath.Dir(socketPath), 0750); err != nil {
		return err
	}
	_ = os.Remove(socketPath)
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return err
	}
	if err := os.Chown(socketPath, uid, -1); err != nil {
		listener.Close()
		return err
	}
	if err := os.Chmod(socketPath, 0600); err != nil {
		listener.Close()
		return err
	}
	for {
		connection, err := listener.Accept()
		if err != nil {
			continue
		}
		go handle(service, connection)
	}
}

func handle(service *nativehelper.Service, connection net.Conn) {
	defer connection.Close()
	decoder := json.NewDecoder(connection)
	decoder.DisallowUnknownFields()
	var request fortihelper.Request
	if err := decoder.Decode(&request); err != nil {
		_ = json.NewEncoder(connection).Encode(fortihelper.Response{State: "failed", Error: "invalid request"})
		return
	}
	_ = json.NewEncoder(connection).Encode(service.Handle(request))
}
