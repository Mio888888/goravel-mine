package services

import (
	"crypto/sha256"
	"encoding/hex"
)

func sha256Hex(payload []byte) string {
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}

func digestBytes(payload []byte) string {
	return "sha256:" + sha256Hex(payload)
}
