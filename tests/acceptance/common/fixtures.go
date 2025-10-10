package common

import (
	"crypto/rand"
	"fmt"
	"testing"
)

// TestData provides common test data generators
type TestData struct {
	t *testing.T
}

// NewTestData creates a new test data generator
func NewTestData(t *testing.T) *TestData {
	return &TestData{t: t}
}

// RandomBytes generates random byte data of specified length
func (td *TestData) RandomBytes(length int) []byte {
	td.t.Helper()
	data := make([]byte, length)
	if _, err := rand.Read(data); err != nil {
		td.t.Fatalf("Failed to generate random bytes: %v", err)
	}
	return data
}

// RandomString generates a random string of specified length
func (td *TestData) RandomString(length int) string {
	td.t.Helper()
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		td.t.Fatalf("Failed to generate random string: %v", err)
	}
	for i := range bytes {
		bytes[i] = charset[bytes[i]%byte(len(charset))]
	}
	return string(bytes)
}

// UniqueKey generates a unique test key with optional prefix
func (td *TestData) UniqueKey(prefix string) string {
	td.t.Helper()
	if prefix == "" {
		prefix = "test"
	}
	return fmt.Sprintf("%s-%s-%s", prefix, td.t.Name(), td.RandomString(8))
}

// KeyValuePairs generates n unique key-value pairs
func (td *TestData) KeyValuePairs(n int) map[string][]byte {
	td.t.Helper()
	pairs := make(map[string][]byte, n)
	for i := 0; i < n; i++ {
		key := td.UniqueKey(fmt.Sprintf("key%d", i))
		value := []byte(fmt.Sprintf("value-%d-%s", i, td.RandomString(8)))
		pairs[key] = value
	}
	return pairs
}

// BinaryData generates test binary data with patterns
func (td *TestData) BinaryData() []byte {
	return []byte{0x00, 0x01, 0x02, 0xFF, 0xFE, 0xFD, 0xDE, 0xAD, 0xBE, 0xEF}
}

// LargeValue generates a large value (default 1MB)
func (td *TestData) LargeValue(sizeMB int) []byte {
	if sizeMB <= 0 {
		sizeMB = 1
	}
	data := make([]byte, sizeMB*1024*1024)
	for i := range data {
		data[i] = byte(i % 256)
	}
	return data
}

// TopicNames generates n unique topic names
func (td *TestData) TopicNames(n int, prefix string) []string {
	td.t.Helper()
	if prefix == "" {
		prefix = "topic"
	}
	topics := make([]string, n)
	for i := 0; i < n; i++ {
		topics[i] = fmt.Sprintf("%s.%s.%d", prefix, td.RandomString(6), i)
	}
	return topics
}

// Payload generates a test message payload
func (td *TestData) Payload(format string, args ...interface{}) []byte {
	td.t.Helper()
	return []byte(fmt.Sprintf(format, args...))
}
