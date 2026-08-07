package runtimepaths

import (
	"path/filepath"
	"runtime"
)

// Paths holds privileged-service and controller filesystem locations for the
// current operating system. Values are absolute layout conventions; callers
// must still create directories and enforce permissions.
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

// Current returns the platform path layout for this process.
func Current() Paths {
	return current()
}

// EngineBundle returns the engine root for the running OS and architecture
// under the given installation base (for example Application Support/Engines).
func EngineBundle(base string) string {
	return filepath.Join(base, runtime.GOOS+"-"+runtime.GOARCH)
}

// ProfileLog returns the session log path for a native profile identifier.
func (paths Paths) ProfileLog(profileID string) string {
	return filepath.Join(paths.LogDirectory, profileID+".log")
}
