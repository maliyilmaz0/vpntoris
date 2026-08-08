//go:build unix

package helperipc

import (
	"encoding/json"
	"net"
	"time"
	"vpntoris-tray/internal/fortihelper"
	"vpntoris-tray/internal/runtimepaths"
)

func Ready() bool {
	info, err := net.DialTimeout("unix", runtimepaths.Current().HelperSocket, 200*time.Millisecond)
	if err != nil {
		return false
	}
	_ = info.Close()
	return true
}
func Call(request fortihelper.Request) (*fortihelper.Response, error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}
	connection, err := net.DialTimeout("unix", runtimepaths.Current().HelperSocket, 2*time.Second)
	if err != nil {
		return nil, err
	}
	defer connection.Close()
	if err := json.NewEncoder(connection).Encode(request); err != nil {
		return nil, err
	}
	var response fortihelper.Response
	if err := json.NewDecoder(connection).Decode(&response); err != nil {
		return nil, err
	}
	return &response, nil
}
