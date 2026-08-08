//go:build linux

package credentials

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// New returns the best available Linux secret store: the desktop Secret
// Service (gnome-keyring / KWallet) via the secret-tool CLI when available,
// otherwise a 0600 file under the user config directory.
func New() Store {
	file := newFileStore()
	if secretToolAvailable() {
		// Hybrid: prefer the Secret Service, fall back to the file so a
		// locked/missing keyring never loses or blocks secrets.
		return &hybridStore{secret: &secretToolStore{}, file: file}
	}
	return file
}

// fileStore persists non-profile secrets under the user config directory.
// Values are never logged by the tray; file mode is 0600.
type fileStore struct {
	mu   sync.Mutex
	path string
}

func newFileStore() *fileStore {
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		dir = filepath.Join(os.TempDir(), "vpntoris")
		log.Printf("credentials: user config dir unavailable, falling back to %s", dir)
	}
	dir = filepath.Join(dir, "VPNToris")
	_ = os.MkdirAll(dir, 0700)
	return &fileStore{path: filepath.Join(dir, "credentials.json")}
}

func (store *fileStore) Write(profile, field, value string) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	data, err := store.load()
	if err != nil {
		return err
	}
	key := profile + "/" + field
	if strings.TrimSpace(value) == "" {
		delete(data, key)
	} else {
		data[key] = value
	}
	return store.save(data)
}

func (store *fileStore) Read(profile, field string) (string, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	data, err := store.load()
	if err != nil {
		return "", err
	}
	value, ok := data[profile+"/"+field]
	if !ok {
		return "", os.ErrNotExist
	}
	return value, nil
}

func (store *fileStore) Delete(profile string) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	data, err := store.load()
	if err != nil {
		return err
	}
	prefix := profile + "/"
	for key := range data {
		if strings.HasPrefix(key, prefix) || key == profile {
			delete(data, key)
		}
	}
	return store.save(data)
}

func (store *fileStore) load() (map[string]string, error) {
	raw, err := os.ReadFile(store.path)
	if os.IsNotExist(err) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	var data map[string]string
	if err := json.Unmarshal(raw, &data); err != nil {
		// A corrupted file must surface an error; returning an empty map
		// here would make the next Write/Delete wipe all stored secrets.
		return nil, err
	}
	if data == nil {
		data = map[string]string{}
	}
	return data, nil
}

func (store *fileStore) save(data map[string]string) error {
	if err := os.MkdirAll(filepath.Dir(store.path), 0700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	tmp := store.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, store.path)
}

// secretToolStore talks to the freedesktop Secret Service (gnome-keyring,
// KWallet, …) through the secret-tool CLI. Secrets travel via stdin/stdout,
// never via process arguments.
type secretToolStore struct {
	// binary locates secret-tool; empty means PATH lookup.
	binary string
}

const secretToolTimeout = 5 * time.Second

var errSecretNotFound = errors.New("secret not found")

// secretToolAvailable reports whether secret-tool exists and a Secret Service
// daemon answers. A lookup for a certainly-missing key exits 1 when the
// service responds but has no match; other failures mean no usable service.
func secretToolAvailable() bool {
	path, err := exec.LookPath("secret-tool")
	if err != nil {
		return false
	}
	_, err = (&secretToolStore{binary: path}).lookup("vpntoris-probe", "vpntoris-probe")
	return err == nil || errors.Is(err, errSecretNotFound)
}

func (store *secretToolStore) run(input string, args ...string) (string, error) {
	binary := store.binary
	if binary == "" {
		binary = "secret-tool"
	}
	ctx, cancel := context.WithTimeout(context.Background(), secretToolTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, args...)
	if input != "" {
		cmd.Stdin = strings.NewReader(input)
	}
	output, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return "", errSecretNotFound
		}
		return "", err
	}
	return strings.TrimSuffix(string(output), "\n"), nil
}

func (store *secretToolStore) lookup(profile, field string) (string, error) {
	return store.run("", "lookup", "service", "vpntoris", "profile", profile, "field", field)
}

func (store *secretToolStore) Write(profile, field, value string) error {
	if strings.TrimSpace(value) == "" {
		_, err := store.run("", "clear", "service", "vpntoris", "profile", profile, "field", field)
		if errors.Is(err, errSecretNotFound) {
			return nil
		}
		return err
	}
	_, err := store.run(value, "store", "--label=VPNToris "+profile+" "+field,
		"service", "vpntoris", "profile", profile, "field", field)
	return err
}

func (store *secretToolStore) Read(profile, field string) (string, error) {
	return store.lookup(profile, field)
}

func (store *secretToolStore) Delete(profile string) error {
	_, err := store.run("", "clear", "service", "vpntoris", "profile", profile)
	if errors.Is(err, errSecretNotFound) {
		return nil
	}
	return err
}

// hybridStore prefers the Secret Service and falls back to the file store on
// any error, so a locked keyring never blocks connect. Reads try both, which
// also migrates secrets written by either backend.
type hybridStore struct {
	secret *secretToolStore
	file   *fileStore
}

func (store *hybridStore) Write(profile, field, value string) error {
	if err := store.secret.Write(profile, field, value); err == nil {
		return nil
	}
	return store.file.Write(profile, field, value)
}

func (store *hybridStore) Read(profile, field string) (string, error) {
	if value, err := store.secret.Read(profile, field); err == nil {
		return value, nil
	}
	return store.file.Read(profile, field)
}

func (store *hybridStore) Delete(profile string) error {
	// Best effort on both backends; only fail when neither worked.
	errSecret := store.secret.Delete(profile)
	errFile := store.file.Delete(profile)
	if errSecret != nil && errFile != nil {
		return errSecret
	}
	return nil
}
