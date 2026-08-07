package main

import (
	"crypto/sha256"
	"encoding/hex"
)

func nativeProfileID(name string) string {
	digest := sha256.Sum256([]byte(name))
	return "profile-" + hex.EncodeToString(digest[:8])
}
