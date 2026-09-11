package utils

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
)

// ComputeSpecHash generates a deterministic SHA-256 hex string representation of the given object.
func ComputeSpecHash(spec interface{}) (string, error) {
	if spec == nil {
		return "", fmt.Errorf("cannot compute hash for nil spec")
	}

	v := reflect.ValueOf(spec)
	if v.Kind() == reflect.Pointer && v.IsNil() {
		return "", fmt.Errorf("cannot compute hash for nil spec after reflect.ValueOf(spec)")
	}

	data, err := json.Marshal(spec)
	if err != nil {
		return "", fmt.Errorf("failed to marshal spec for hashing: %w", err)
	}

	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}
