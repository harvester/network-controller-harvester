package hostnetworkconfig

import (
	"testing"
	"time"
)

func TestLocalHostNetworkConfigState_IsReady(t *testing.T) {
	targetHash := "hash-12345"
	wrongHash := "hash-99999"

	tests := []struct {
		name       string
		state      LocalHostNetworkConfigState
		targetHash string
		ttl        time.Duration
		want       bool
	}{
		{
			name: "Valid StateReady with non-zero TTL before expiration",
			state: LocalHostNetworkConfigState{
				ConfigHash:    targetHash,
				Status:        StateReady,
				LastValidated: time.Now().Add(-5 * time.Minute),
			},
			targetHash: targetHash,
			ttl:        10 * time.Minute,
			want:       true,
		},
		{
			name: "Expired StateReady with non-zero TTL",
			state: LocalHostNetworkConfigState{
				ConfigHash:    targetHash,
				Status:        StateReady,
				LastValidated: time.Now().Add(-15 * time.Minute),
			},
			targetHash: targetHash,
			ttl:        10 * time.Minute,
			want:       false,
		},
		{
			name: "StateReady with zero TTL bypasses duration check (old timestamp)",
			state: LocalHostNetworkConfigState{
				ConfigHash:    targetHash,
				Status:        StateReady,
				LastValidated: time.Now().Add(-24 * time.Hour),
			},
			targetHash: targetHash,
			ttl:        0,
			want:       true,
		},
		{
			name: "Mismatched ConfigHash",
			state: LocalHostNetworkConfigState{
				ConfigHash:    targetHash,
				Status:        StateReady,
				LastValidated: time.Now(),
			},
			targetHash: wrongHash,
			ttl:        10 * time.Minute,
			want:       false,
		},
		{
			name: "Incorrect Status (StateSetting instead of StateReady)",
			state: LocalHostNetworkConfigState{
				ConfigHash:    targetHash,
				Status:        StateSetting,
				LastValidated: time.Now(),
			},
			targetHash: targetHash,
			ttl:        10 * time.Minute,
			want:       false,
		},
		{
			name: "Incorrect Status (StateRemoved instead of StateReady)",
			state: LocalHostNetworkConfigState{
				ConfigHash:    targetHash,
				Status:        StateRemoved,
				LastValidated: time.Now(),
			},
			targetHash: targetHash,
			ttl:        10 * time.Minute,
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.state.IsReady(tt.targetHash, tt.ttl)
			if got != tt.want {
				t.Errorf("IsReady() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLocalHostNetworkConfigState_IsRemoved(t *testing.T) {
	targetHash := "hash-12345"
	wrongHash := "hash-99999"

	tests := []struct {
		name       string
		state      LocalHostNetworkConfigState
		targetHash string
		ttl        time.Duration
		want       bool
	}{
		{
			name: "Valid StateRemoved with non-zero TTL before expiration",
			state: LocalHostNetworkConfigState{
				ConfigHash:    targetHash,
				Status:        StateRemoved,
				LastValidated: time.Now().Add(-2 * time.Minute),
			},
			targetHash: targetHash,
			ttl:        5 * time.Minute,
			want:       true,
		},
		{
			name: "Expired StateRemoved with non-zero TTL",
			state: LocalHostNetworkConfigState{
				ConfigHash:    targetHash,
				Status:        StateRemoved,
				LastValidated: time.Now().Add(-10 * time.Minute),
			},
			targetHash: targetHash,
			ttl:        5 * time.Minute,
			want:       false,
		},
		{
			name: "StateRemoved with zero TTL bypasses duration check (old timestamp)",
			state: LocalHostNetworkConfigState{
				ConfigHash:    targetHash,
				Status:        StateRemoved,
				LastValidated: time.Now().Add(-48 * time.Hour),
			},
			targetHash: targetHash,
			ttl:        0,
			want:       true,
		},
		{
			name: "Mismatched ConfigHash",
			state: LocalHostNetworkConfigState{
				ConfigHash:    targetHash,
				Status:        StateRemoved,
				LastValidated: time.Now(),
			},
			targetHash: wrongHash,
			ttl:        5 * time.Minute,
			want:       false,
		},
		{
			name: "Incorrect Status (StateRemoving instead of StateRemoved)",
			state: LocalHostNetworkConfigState{
				ConfigHash:    targetHash,
				Status:        StateRemoving,
				LastValidated: time.Now(),
			},
			targetHash: targetHash,
			ttl:        5 * time.Minute,
			want:       false,
		},
		{
			name: "Incorrect Status (StateReady instead of StateRemoved)",
			state: LocalHostNetworkConfigState{
				ConfigHash:    targetHash,
				Status:        StateReady,
				LastValidated: time.Now(),
			},
			targetHash: targetHash,
			ttl:        5 * time.Minute,
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.state.IsRemoved(tt.targetHash, tt.ttl)
			if got != tt.want {
				t.Errorf("IsRemoved() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLocalHostNetworkConfigStateManager_Get_IsReady_Integration(t *testing.T) {
	nodeName := "node-1"
	targetHash := "hash-abc"
	ttl := 10 * time.Minute

	mgr := NewLocalHostNetworkConfigStateManager(ttl)

	// Case 1: Node does not exist
	if _, exists := mgr.Get(nodeName); exists {
		t.Fatalf("expected node %s to not exist in manager", nodeName)
	}

	// Case 2: Store ready state and query via Get()
	state := NewLocalHostNetworkConfigState(targetHash, StateReady)
	mgr.CreateOrUpdate(nodeName, state)

	fetchedState, exists := mgr.Get(nodeName)
	if !exists {
		t.Fatalf("expected node %s to exist in manager", nodeName)
	}

	if !fetchedState.IsReady(targetHash, mgr.TTL()) {
		t.Errorf("expected fetchedState.IsReady() to be true")
	}

	// Case 3: Update to removed state and verify
	removedState := NewLocalHostNetworkConfigState(targetHash, StateRemoved)
	mgr.CreateOrUpdate(nodeName, removedState)

	fetchedState, _ = mgr.Get(nodeName)
	if fetchedState.IsReady(targetHash, mgr.TTL()) {
		t.Errorf("expected fetchedState.IsReady() to be false after updating to StateRemoved")
	}
	if !fetchedState.IsRemoved(targetHash, mgr.TTL()) {
		t.Errorf("expected fetchedState.IsRemoved() to be true")
	}
}
