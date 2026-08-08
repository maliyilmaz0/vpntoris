//go:build windows

package helperipc

import (
	"encoding/json"
	"github.com/Microsoft/go-winio"
	"net"
	"vpntoris-tray/internal/fortihelper"
	"vpntoris-tray/internal/nativehelper"
	"vpntoris-tray/internal/runtimepaths"
)

func ServePipe(service *nativehelper.Service, paths runtimepaths.Paths) error {
	config := &winio.PipeConfig{
		SecurityDescriptor: "D:P(A;;GA;;;BA)(A;;GA;;;SY)(A;;GRGW;;;AU)",
		MessageMode:        false,
		InputBufferSize:    65536,
		OutputBufferSize:   65536,
	}
	listener, err := winio.ListenPipe(paths.HelperSocket, config)
	if err != nil {
		return err
	}
	for {
		connection, err := listener.Accept()
		if err != nil {
			continue
		}
		go handleWindows(service, connection)
	}
}
func handleWindows(service *nativehelper.Service, connection net.Conn) {
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
