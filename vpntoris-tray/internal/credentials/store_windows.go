//go:build windows

package credentials

import (
	"strings"

	"github.com/danieljoos/wincred"
)

const windowsTargetPrefix = "VPNToris/"

// New returns a Windows Credential Manager backed store.
func New() Store {
	return windowsStore{}
}

type windowsStore struct{}

func (windowsStore) Write(profile, field, value string) error {
	target := targetName(profile, field)
	if value == "" {
		return deleteTarget(target)
	}
	credential := wincred.NewGenericCredential(target)
	credential.CredentialBlob = []byte(value)
	credential.Persist = wincred.PersistLocalMachine
	return credential.Write()
}

func (windowsStore) Read(profile, field string) (string, error) {
	credential, err := wincred.GetGenericCredential(targetName(profile, field))
	if err != nil {
		return "", err
	}
	return string(credential.CredentialBlob), nil
}

func (windowsStore) Delete(profile string) error {
	_ = deleteTarget(targetName(profile, "password"))
	_ = deleteTarget(targetName(profile, "psk"))
	return nil
}

func deleteTarget(target string) error {
	credential, err := wincred.GetGenericCredential(target)
	if err != nil {
		return nil
	}
	return credential.Delete()
}

func targetName(profile, field string) string {
	return windowsTargetPrefix + strings.TrimSpace(profile) + "/" + strings.TrimSpace(field)
}
