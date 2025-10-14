package common

// ClaimCheckMessage represents metadata for a claim check reference
// Used by both producer (to create claims) and consumer (to resolve claims)
type ClaimCheckMessage struct {
	ClaimID      string `json:"claim_id"`
	Bucket       string `json:"bucket"`
	ObjectKey    string `json:"object_key"`
	OriginalSize int    `json:"original_size"`
	Compression  string `json:"compression"`
	ContentType  string `json:"content_type"`
	Checksum     string `json:"checksum"` // SHA-256 hex
	ExpiresAt    int64  `json:"expires_at,omitempty"`
}
