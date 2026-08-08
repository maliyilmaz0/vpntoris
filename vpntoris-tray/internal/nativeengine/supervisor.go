package nativeengine

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type EngineStart struct {
	ManifestPath string
	Arguments    []string
	Environment  map[string]string
	Input        io.Reader
	LogPath      string
}
type EngineProcess struct {
	Manifest EngineManifest
	PID      int
	Started  time.Time
	command  *exec.Cmd
	log      *os.File
	done     chan error
	stopOnce sync.Once
}
type Supervisor struct {
	root string
}

func NewSupervisor(root string) (*Supervisor, error) {
	path, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if info, err := os.Stat(path); err != nil || !info.IsDir() {
		return nil, fmt.Errorf("engine bundle directory is unavailable")
	}
	return &Supervisor{root: path}, nil
}
func (supervisor *Supervisor) Start(ctx context.Context, start EngineStart) (*EngineProcess, error) {
	manifest, executable, err := LoadEngineManifest(supervisor.root, start.ManifestPath)
	if err != nil {
		return nil, err
	}
	if start.LogPath == "" {
		return nil, fmt.Errorf("engine log path is required")
	}
	if err := os.MkdirAll(filepath.Dir(start.LogPath), 0700); err != nil {
		return nil, err
	}
	logFile, err := os.OpenFile(start.LogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return nil, err
	}
	command := exec.CommandContext(ctx, executable, start.Arguments...)
	command.Stdin = start.Input
	command.Stdout = logFile
	command.Stderr = logFile
	command.Env = controlledEnvironment(start.Environment)
	command.Dir = supervisor.root
	configureProcess(command)
	if err := command.Start(); err != nil {
		logFile.Close()
		return nil, err
	}
	process := &EngineProcess{Manifest: *manifest, PID: command.Process.Pid, Started: time.Now().UTC(), command: command, log: logFile, done: make(chan error, 1)}
	go func() {
		process.done <- command.Wait()
		logFile.Close()
	}()
	return process, nil
}
func (process *EngineProcess) Wait() error {
	return <-process.done
}
func (process *EngineProcess) Stop(timeout time.Duration) error {
	var stopError error
	process.stopOnce.Do(func() {
		if process.command == nil || process.command.Process == nil {
			return
		}
		stopError = terminateProcess(process.command.Process)
		if stopError != nil {
			return
		}
		select {
		case <-process.done:
		case <-time.After(timeout):
			stopError = process.command.Process.Kill()
		}
	})
	return stopError
}
func controlledEnvironment(values map[string]string) []string {
	allowed := []string{"LANG", "LC_ALL", "SSL_CERT_FILE", "SSL_CERT_DIR", "SYSTEMROOT", "WINDIR"}
	environment := []string{}
	for _, key := range allowed {
		if value := os.Getenv(key); value != "" {
			environment = append(environment, key+"="+value)
		}
	}
	for key, value := range values {
		if safeIdentifier.MatchString(key) && !strings.ContainsRune(value, '\x00') {
			environment = append(environment, key+"="+value)
		}
	}
	return environment
}
