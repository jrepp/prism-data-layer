package interfaces_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCase represents a single table-driven test case
type TestCase struct {
	Name        string
	Setup       func(t *testing.T, driver KeyValueBasicDriver) // Optional setup
	Run         func(t *testing.T, driver KeyValueBasicDriver) // Test logic
	Verify      func(t *testing.T, driver KeyValueBasicDriver) // Optional verification
	Cleanup     func(t *testing.T, driver KeyValueBasicDriver) // Optional cleanup
	SkipBackend map[string]bool                                // Backends to skip
}

// RandomDataGenerator generates random test data for property-based testing
type RandomDataGenerator struct {
	seed int64
}

// NewRandomDataGenerator creates a new random data generator
func NewRandomDataGenerator() *RandomDataGenerator {
	return &RandomDataGenerator{}
}

// RandomString generates a random string of specified length
func (g *RandomDataGenerator) RandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		b[i] = charset[n.Int64()]
	}
	return string(b)
}

// RandomKey generates a random key with test prefix
func (g *RandomDataGenerator) RandomKey(testName string) string {
	// Sanitize test name for use in key
	sanitized := strings.ReplaceAll(testName, " ", "_")
	sanitized = strings.ReplaceAll(sanitized, "/", "_")
	return fmt.Sprintf("test:%s:%s", sanitized, g.RandomString(8))
}

// RandomBytes generates random byte slice
func (g *RandomDataGenerator) RandomBytes(length int) []byte {
	b := make([]byte, length)
	rand.Read(b)
	return b
}

// RandomHex generates random hex string
func (g *RandomDataGenerator) RandomHex(length int) string {
	b := make([]byte, length/2)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// RandomInt generates random integer in range [min, max]
func (g *RandomDataGenerator) RandomInt(min, max int) int {
	diff := max - min
	n, _ := rand.Int(rand.Reader, big.NewInt(int64(diff)))
	return min + int(n.Int64())
}

// TestData represents test data for verification
type TestData struct {
	Key   string
	Value []byte
}

// KeyValueTestSuite defines a complete test suite for KeyValue interfaces
type KeyValueTestSuite struct {
	Name      string
	TestCases []TestCase
}

// GetKeyValueBasicTestSuite returns the comprehensive test suite for keyvalue_basic interface
func GetKeyValueBasicTestSuite() KeyValueTestSuite {
	gen := NewRandomDataGenerator()

	return KeyValueTestSuite{
		Name: "KeyValueBasicInterface",
		TestCases: []TestCase{
			// Basic operations
			{
				Name: "Set_Get_Random_Data",
				Run: func(t *testing.T, driver KeyValueBasicDriver) {
					key := gen.RandomKey(t.Name())
					value := []byte(gen.RandomString(100))

					err := driver.Set(key, value, 0)
					require.NoError(t, err, "Set should succeed")

					retrieved, found, err := driver.Get(key)
					require.NoError(t, err, "Get should not error")
					assert.True(t, found, "Key should be found")
					assert.Equal(t, value, retrieved, "Retrieved value should match")
				},
			},
			{
				Name: "Set_Get_Binary_Random_Data",
				Run: func(t *testing.T, driver KeyValueBasicDriver) {
					key := gen.RandomKey(t.Name())
					value := gen.RandomBytes(256)

					err := driver.Set(key, value, 0)
					require.NoError(t, err, "Set should succeed")

					retrieved, found, err := driver.Get(key)
					require.NoError(t, err, "Get should not error")
					assert.True(t, found, "Key should be found")
					assert.Equal(t, value, retrieved, "Retrieved binary value should match")
				},
			},
			{
				Name: "Multiple_Random_Keys",
				Run: func(t *testing.T, driver KeyValueBasicDriver) {
					numKeys := gen.RandomInt(10, 50)
					testData := make([]TestData, numKeys)

					// Write random data
					for i := 0; i < numKeys; i++ {
						testData[i] = TestData{
							Key:   gen.RandomKey(t.Name()),
							Value: []byte(gen.RandomString(gen.RandomInt(10, 200))),
						}
						err := driver.Set(testData[i].Key, testData[i].Value, 0)
						require.NoError(t, err, "Set should succeed for key %d", i)
					}

					// Verify all data can be read back
					for i, data := range testData {
						retrieved, found, err := driver.Get(data.Key)
						require.NoError(t, err, "Get should not error for key %d", i)
						assert.True(t, found, "Key %d should be found", i)
						assert.Equal(t, data.Value, retrieved, "Value %d should match", i)
					}
				},
			},
			{
				Name: "Overwrite_With_Random_Data",
				Run: func(t *testing.T, driver KeyValueBasicDriver) {
					key := gen.RandomKey(t.Name())

					// Write initial value
					value1 := []byte(gen.RandomString(50))
					err := driver.Set(key, value1, 0)
					require.NoError(t, err, "First set should succeed")

					// Overwrite with different value
					value2 := []byte(gen.RandomString(75))
					err = driver.Set(key, value2, 0)
					require.NoError(t, err, "Second set should succeed")

					// Verify latest value
					retrieved, found, err := driver.Get(key)
					require.NoError(t, err, "Get should not error")
					assert.True(t, found, "Key should be found")
					assert.Equal(t, value2, retrieved, "Should retrieve latest value")
					assert.NotEqual(t, value1, retrieved, "Should not retrieve old value")
				},
			},
			{
				Name: "Delete_Random_Keys",
				Run: func(t *testing.T, driver KeyValueBasicDriver) {
					numKeys := gen.RandomInt(5, 15)
					keys := make([]string, numKeys)

					// Create keys
					for i := 0; i < numKeys; i++ {
						keys[i] = gen.RandomKey(t.Name())
						err := driver.Set(keys[i], []byte(gen.RandomString(20)), 0)
						require.NoError(t, err, "Set should succeed for key %d", i)
					}

					// Delete random subset
					deleteCount := gen.RandomInt(1, numKeys)
					deletedKeys := make(map[string]bool)
					for i := 0; i < deleteCount; i++ {
						keyToDelete := keys[i]
						err := driver.Delete(keyToDelete)
						require.NoError(t, err, "Delete should succeed")
						deletedKeys[keyToDelete] = true
					}

					// Verify deleted keys are gone, others remain
					for _, key := range keys {
						_, found, err := driver.Get(key)
						require.NoError(t, err, "Get should not error")
						if deletedKeys[key] {
							assert.False(t, found, "Deleted key should not be found: %s", key)
						} else {
							assert.True(t, found, "Non-deleted key should be found: %s", key)
						}
					}
				},
			},
			{
				Name: "Exists_Random_Keys",
				Run: func(t *testing.T, driver KeyValueBasicDriver) {
					existingKey := gen.RandomKey(t.Name() + "_existing")
					nonExistingKey := gen.RandomKey(t.Name() + "_nonexisting")

					// Create one key
					err := driver.Set(existingKey, []byte("value"), 0)
					require.NoError(t, err, "Set should succeed")

					// Verify exists
					exists, err := driver.Exists(existingKey)
					require.NoError(t, err, "Exists should not error")
					assert.True(t, exists, "Existing key should exist")

					// Verify non-existing doesn't exist
					exists, err = driver.Exists(nonExistingKey)
					require.NoError(t, err, "Exists should not error")
					assert.False(t, exists, "Non-existing key should not exist")
				},
			},
			{
				Name: "Large_Random_Values",
				Run: func(t *testing.T, driver KeyValueBasicDriver) {
					// Test various sizes
					sizes := []int{
						gen.RandomInt(1024, 10*1024),        // 1-10KB
						gen.RandomInt(100*1024, 500*1024),   // 100-500KB
						gen.RandomInt(1024*1024, 2*1024*1024), // 1-2MB
					}

					for _, size := range sizes {
						key := gen.RandomKey(t.Name())
						value := gen.RandomBytes(size)

						err := driver.Set(key, value, 0)
						require.NoError(t, err, "Set should succeed for %d byte value", size)

						retrieved, found, err := driver.Get(key)
						require.NoError(t, err, "Get should not error for %d byte value", size)
						assert.True(t, found, "Key should be found for %d byte value", size)
						assert.Equal(t, len(value), len(retrieved), "Retrieved size should match for %d byte value", size)
						assert.Equal(t, value, retrieved, "Retrieved value should match for %d byte value", size)
					}
				},
			},
			{
				Name: "Empty_And_Null_Values",
				Run: func(t *testing.T, driver KeyValueBasicDriver) {
					emptyKey := gen.RandomKey(t.Name() + "_empty")

					// Store empty value
					err := driver.Set(emptyKey, []byte(""), 0)
					require.NoError(t, err, "Set empty value should succeed")

					retrieved, found, err := driver.Get(emptyKey)
					require.NoError(t, err, "Get should not error")
					assert.True(t, found, "Empty value key should be found")
					assert.Equal(t, []byte(""), retrieved, "Retrieved empty value should match")
				},
			},
			{
				Name: "Special_Characters_In_Keys",
				Run: func(t *testing.T, driver KeyValueBasicDriver) {
					// Test keys with special characters
					specialKeys := []string{
						gen.RandomKey("test:with:colons"),
						gen.RandomKey("test-with-dashes"),
						gen.RandomKey("test_with_underscores"),
						gen.RandomKey("test.with.dots"),
						gen.RandomKey("test/with/slashes"),
					}

					for _, key := range specialKeys {
						value := []byte(gen.RandomString(20))
						err := driver.Set(key, value, 0)
						require.NoError(t, err, "Set should succeed for key: %s", key)

						retrieved, found, err := driver.Get(key)
						require.NoError(t, err, "Get should not error for key: %s", key)
						assert.True(t, found, "Key should be found: %s", key)
						assert.Equal(t, value, retrieved, "Value should match for key: %s", key)
					}
				},
			},
			{
				Name: "Rapid_Sequential_Operations",
				Run: func(t *testing.T, driver KeyValueBasicDriver) {
					key := gen.RandomKey(t.Name())
					iterations := gen.RandomInt(50, 100)

					// Rapid updates
					var lastValue []byte
					for i := 0; i < iterations; i++ {
						value := []byte(fmt.Sprintf("iteration_%d_%s", i, gen.RandomString(10)))
						err := driver.Set(key, value, 0)
						require.NoError(t, err, "Set should succeed on iteration %d", i)
						lastValue = value
					}

					// Verify final value
					retrieved, found, err := driver.Get(key)
					require.NoError(t, err, "Get should not error")
					assert.True(t, found, "Key should be found")
					assert.Equal(t, lastValue, retrieved, "Should have latest value")
				},
			},
		},
	}
}

// RunTestSuite runs a test suite against all configured backends
func RunTestSuite(t *testing.T, suite KeyValueTestSuite, backends []BackendDriverSetup) {
	for _, backend := range backends {
		t.Run(backend.Name, func(t *testing.T) {
			ctx := context.Background()
			driver, cleanup := backend.SetupFunc(t, ctx)
			defer cleanup()

			for _, tc := range suite.TestCases {
				// Skip if backend doesn't support this test
				if tc.SkipBackend != nil && tc.SkipBackend[backend.Name] {
					t.Logf("Skipping test %s for backend %s", tc.Name, backend.Name)
					continue
				}

				t.Run(tc.Name, func(t *testing.T) {
					// Setup
					if tc.Setup != nil {
						tc.Setup(t, driver)
					}

					// Run test
					tc.Run(t, driver)

					// Verify
					if tc.Verify != nil {
						tc.Verify(t, driver)
					}

					// Cleanup
					if tc.Cleanup != nil {
						tc.Cleanup(t, driver)
					}
				})
			}
		})
	}
}

// TestKeyValueBasicInterface_TableDriven runs comprehensive table-driven tests
func TestKeyValueBasicInterface_TableDriven(t *testing.T) {
	suite := GetKeyValueBasicTestSuite()
	backends := GetStandardBackends()

	t.Logf("Running %d test cases against %d backends", len(suite.TestCases), len(backends))
	RunTestSuite(t, suite, backends)
}
