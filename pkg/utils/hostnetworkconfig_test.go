package utils

import (
	"strings"
	"testing"
)

type DummySpec struct {
	ClusterNetwork string            `json:"clusterNetwork"`
	VlanID         int               `json:"vlanId"`
	NodeSelector   map[string]string `json:"nodeSelector"`
	HostIPs        []string          `json:"hostIPs"`
	Underlay       bool              `json:"underlay"`
}

func TestComputeSpecHash(t *testing.T) {
	baseSpec := DummySpec{
		ClusterNetwork: "vlan",
		VlanID:         100,
		NodeSelector: map[string]string{
			"kubernetes.io/hostname":      "node1",
			"topology.kubernetes.io/zone": "zone-a",
		},
		HostIPs:  []string{"192.168.1.10/24"},
		Underlay: true,
	}

	t.Run("valid struct returns non-empty hash", func(t *testing.T) {
		hash, err := ComputeSpecHash(baseSpec)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if len(hash) != 64 {
			t.Errorf("expected 64-character SHA-256 hex string, got length %d (%s)", len(hash), hash)
		}
	})

	t.Run("untyped nil spec returns error", func(t *testing.T) {
		hash, err := ComputeSpecHash(nil)
		if err == nil {
			t.Fatal("expected error for untyped nil spec, got nil")
		}
		if hash != "" {
			t.Errorf("expected empty hash string, got: %s", hash)
		}
		if !strings.Contains(err.Error(), "cannot compute hash for nil spec") {
			t.Errorf("unexpected error message: %v", err)
		}
	})

	t.Run("typed nil pointer spec returns error", func(t *testing.T) {
		var nilSpec *DummySpec
		hash, err := ComputeSpecHash(nilSpec)
		if err == nil {
			t.Fatal("expected error for typed nil pointer spec, got nil")
		}
		if hash != "" {
			t.Errorf("expected empty hash string, got: %s", hash)
		}
		if !strings.Contains(err.Error(), "cannot compute hash for nil spec") {
			t.Errorf("unexpected error message: %v", err)
		}
	})

	t.Run("deterministic hash across identical structs", func(t *testing.T) {
		spec1 := baseSpec
		spec2 := baseSpec

		hash1, err1 := ComputeSpecHash(spec1)
		hash2, err2 := ComputeSpecHash(spec2)

		if err1 != nil || err2 != nil {
			t.Fatalf("unexpected errors: err1=%v, err2=%v", err1, err2)
		}
		if hash1 != hash2 {
			t.Errorf("expected hashes to match, got %s and %s", hash1, hash2)
		}
	})

	t.Run("deterministic hash despite map key iteration order", func(t *testing.T) {
		spec1 := DummySpec{
			NodeSelector: map[string]string{"a": "1", "b": "2", "c": "3"},
		}
		spec2 := DummySpec{
			NodeSelector: map[string]string{"c": "3", "a": "1", "b": "2"},
		}

		hash1, err1 := ComputeSpecHash(spec1)
		hash2, err2 := ComputeSpecHash(spec2)

		if err1 != nil || err2 != nil {
			t.Fatalf("unexpected errors: err1=%v, err2=%v", err1, err2)
		}
		if hash1 != hash2 {
			t.Errorf("expected identical hashes regardless of map construction order, got %s and %s", hash1, hash2)
		}
	})

	t.Run("different spec fields produce different hashes", func(t *testing.T) {
		modifiedSpec := baseSpec
		modifiedSpec.VlanID = 200

		hashBase, err1 := ComputeSpecHash(baseSpec)
		hashMod, err2 := ComputeSpecHash(modifiedSpec)

		if err1 != nil || err2 != nil {
			t.Fatalf("unexpected errors: err1=%v, err2=%v", err1, err2)
		}
		if hashBase == hashMod {
			t.Errorf("expected different hashes for different spec fields, but got same hash: %s", hashBase)
		}
	})
}
