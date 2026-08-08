//go:build windows

package helperipc

import (
	"encoding/json"
	"github.com/Microsoft/go-winio"
	"time"
	"vpntoris-tray/internal/fortihelper"
	"vpntoris-tray/internal/runtimepaths"
)

func Ready() bool {
	timeout := 200 * time.Millisecond
	connection, err := winio.DialPipe(runtimepaths.Current().HelperSocket, &timeout)
	if err != nil {
		return false
	}
	_ = connection.Close()
	return true
}
func Call(request fortihelper.Request) (*fortihelper.Response, error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}
	timeout := 2 * time.Second
	connection, err := winio.DialPipe(runtimepaths.Current().HelperSocket, &timeout)
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
