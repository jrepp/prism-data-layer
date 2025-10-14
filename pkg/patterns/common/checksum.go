package common

import (
	"crypto/sha256"
	"encoding/hex"
)

// CalculateSHA256 computes the SHA-256 checksum of data and returns it as a hex string
func CalculateSHA256(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}
