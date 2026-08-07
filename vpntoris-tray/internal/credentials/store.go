package credentials

// Store persists non-profile secrets for the current user.
type Store interface {
	Write(profile, field, value string) error
	Read(profile, field string) (string, error)
	Delete(profile string) error
}
