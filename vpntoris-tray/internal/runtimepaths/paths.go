package runtimepaths

import (
	"path/filepath"
	"runtime"
)

type Paths struct {
	Platform         string `json:"platform"`
	Architecture     string `json:"architecture"`
	StateDirectory   string `json:"stateDirectory"`
	RuntimeDirectory string `json:"runtimeDirectory"`
	LogDirectory     string `json:"logDirectory"`
	EngineDirectory  string `json:"engineDirectory"`
	HelperSocket     string `json:"helperSocket"`
	RouterSocket     string `json:"routerSocket,omitempty"`
}

func Current() Paths {
	return current()
}
func EngineBundle(base string) string {
	return filepath.Join(base, runtime.GOOS+"-"+runtime.GOARCH)
}
func (paths Paths) ProfileLog(profileID string) string {
	return filepath.Join(paths.LogDirectory, profileID+".log")
}
