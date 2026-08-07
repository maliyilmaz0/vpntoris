package nativehelper

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// engineSideBinary resolves a helper shipped next to an engine executable.
// On Windows, prefer a .exe suffix and do not require Unix execute bits.
func engineSideBinary(dir, name string) (string, error) {
	candidates := []string{filepath.Join(dir, name)}
	if runtime.GOOS == "windows" {
		candidates = []string{
			filepath.Join(dir, name+".exe"),
			filepath.Join(dir, name),
		}
	}
	for _, path := range candidates {
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			continue
		}
		if runtime.GOOS != "windows" && info.Mode()&0111 == 0 {
			continue
		}
		return path, nil
	}
	return "", fmt.Errorf("%s not found beside engine", name)
}
