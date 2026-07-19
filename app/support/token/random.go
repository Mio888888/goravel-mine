package token

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

func RandomHex(size int) string {
	return RandomHexWithFallback(size, func() string {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	})
}

func RandomHexWithFallback(size int, fallback func() string) string {
	if size < 1 {
		size = 16
	}
	bytes := make([]byte, size)
	if _, err := rand.Read(bytes); err != nil {
		if fallback != nil {
			return fallback()
		}
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(bytes)
}
