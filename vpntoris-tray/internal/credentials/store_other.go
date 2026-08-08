//go:build !windows && !linux

package credentials

import "fmt"

type memoryStore struct {
	values map[string]string
}

// New returns an in-memory store for non-Windows, non-Linux platforms (e.g. tests on macOS helpers).
func New() Store {
	return &memoryStore{values: map[string]string{}}
}

func (store *memoryStore) Write(profile, field, value string) error {
	key := profile + "/" + field
	if value == "" {
		delete(store.values, key)
		return nil
	}
	store.values[key] = value
	return nil
}

func (store *memoryStore) Read(profile, field string) (string, error) {
	value, ok := store.values[profile+"/"+field]
	if !ok {
		return "", fmt.Errorf("credential not found")
	}
	return value, nil
}

func (store *memoryStore) Delete(profile string) error {
	for key := range store.values {
		if len(key) >= len(profile) && key[:len(profile)] == profile {
			delete(store.values, key)
		}
	}
	return nil
}
