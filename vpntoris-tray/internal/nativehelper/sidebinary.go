package nativehelper

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

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

func spaceFreeBinary(runtimeDir, target string) (string, error) {
	if !strings.ContainsAny(target, " \t") {
		return target, nil
	}
	link := filepath.Join(runtimeDir, filepath.Base(target))
	if err := os.Remove(link); err != nil && !os.IsNotExist(err) {
		return "", err
	}
	if err := os.Symlink(target, link); err != nil {
		return "", err
	}
	return link, nil
}
