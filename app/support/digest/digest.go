package digest

import (
	"crypto/sha256"
	"encoding/hex"
)

func SHA256Hex(payload []byte) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func SHA256(payload []byte) string {
	return "sha256:" + SHA256Hex(payload)
}
