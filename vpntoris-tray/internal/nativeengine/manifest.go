package nativeengine

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type EngineManifest struct {
	ID           string   `json:"id"`
	Protocol     string   `json:"protocol"`
	Version      string   `json:"version"`
	OS           string   `json:"os"`
	Architecture string   `json:"architecture"`
	Executable   string   `json:"executable"`
	SHA256       string   `json:"sha256"`
	License      string   `json:"license"`
	Capabilities []string `json:"capabilities"`
}

func LoadEngineManifest(root, manifestPath string) (*EngineManifest, string, error) {
	rootPath, err := filepath.Abs(root)
	if err != nil {
		return nil, "", err
	}
	rootPath, err = filepath.EvalSymlinks(rootPath)
	if err != nil {
		return nil, "", err
	}
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, "", err
	}
	var manifest EngineManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, "", err
	}
	if !safeIdentifier.MatchString(manifest.ID) || manifest.Protocol == "" || manifest.Version == "" || manifest.OS != runtime.GOOS || manifest.Architecture != runtime.GOARCH || len(manifest.SHA256) != 64 {
		return nil, "", fmt.Errorf("invalid or incompatible engine manifest")
	}
	executablePath := filepath.Join(rootPath, filepath.Clean(manifest.Executable))
	resolvedExecutable, err := filepath.EvalSymlinks(executablePath)
	if err != nil {
		return nil, "", err
	}
	relative, err := filepath.Rel(rootPath, resolvedExecutable)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return nil, "", fmt.Errorf("engine executable escapes bundle root")
	}
	digest, err := fileSHA256(resolvedExecutable)
	if err != nil {
		return nil, "", err
	}
	if !strings.EqualFold(digest, manifest.SHA256) {
		return nil, "", fmt.Errorf("engine digest mismatch")
	}
	return &manifest, resolvedExecutable, nil
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
