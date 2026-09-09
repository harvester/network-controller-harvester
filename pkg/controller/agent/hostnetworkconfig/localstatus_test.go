package hostnetworkconfig

import (
	"os"
	"testing"
	"time"

	"github.com/harvester/harvester-network-controller/pkg/utils"
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

	mgr := NewLocalHostNetworkConfigStateManager(ttl, false)

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

func TestLocalHostNetworkConfigStateManager_Disabled(t *testing.T) {
	nodeName := "node-1"
	targetHash := "hash-abc"
	ttl := 10 * time.Minute

	t.Run("Disabled manager blocks writes and returns empty on reads", func(t *testing.T) {
		mgr := NewLocalHostNetworkConfigStateManager(ttl, true)

		if !mgr.Disabled() {
			t.Errorf("expected Disabled() to be true")
		}

		// CreateOrUpdate should perform a no-op and return false
		state := NewLocalHostNetworkConfigState(targetHash, StateReady)
		created := mgr.CreateOrUpdate(nodeName, state)
		if created {
			t.Errorf("expected CreateOrUpdate() to return false when disabled")
		}

		// Get should return empty state and exists=false
		fetchedState, exists := mgr.Get(nodeName)
		if exists {
			t.Errorf("expected Get() exists to be false when disabled")
		}

		// IsReady / IsRemoved helper checks on fetched state should evaluate to false
		if fetchedState.IsReady(targetHash, mgr.TTL()) {
			t.Errorf("expected IsReady() to be false on empty state from disabled manager")
		}
		if fetchedState.IsRemoved(targetHash, mgr.TTL()) {
			t.Errorf("expected IsRemoved() to be false on empty state from disabled manager")
		}

		// Delete should execute safely as a no-op
		mgr.Delete(nodeName)
	})

	t.Run("Disabled manager String output", func(t *testing.T) {
		mgrDisabled := NewLocalHostNetworkConfigStateManager(ttl, true)
		expectedDisabledStr := "LocalHostNetworkConfigStateManager{disabled: true}"
		if got := mgrDisabled.String(); got != expectedDisabledStr {
			t.Errorf("String() = %q, want %q", got, expectedDisabledStr)
		}

		mgrEnabled := NewLocalHostNetworkConfigStateManager(ttl, false)
		expectedEnabledStr := "LocalHostNetworkConfigStateManager{disabled: false, ttl: 10m0s, count: 0}"
		if got := mgrEnabled.String(); got != expectedEnabledStr {
			t.Errorf("String() = %q, want %q", got, expectedEnabledStr)
		}
	})
}

func Test_getDisableFromEnvOrDefault(t *testing.T) {
	tests := []struct {
		name   string
		envVal string
		setEnv bool
		want   bool
	}{
		{
			name:   "Env unset defaults to false",
			setEnv: false,
			want:   false,
		},
		{
			name:   "Env empty defaults to false",
			envVal: "",
			setEnv: true,
			want:   false,
		},
		{
			name:   "Env set to 'true'",
			envVal: "true",
			setEnv: true,
			want:   true,
		},
		{
			name:   "Env set to '1'",
			envVal: "1",
			setEnv: true,
			want:   true,
		},
		{
			name:   "Env set to 'false'",
			envVal: "false",
			setEnv: true,
			want:   false,
		},
		{
			name:   "Env set to invalid boolean value defaults to false",
			envVal: "not-a-bool",
			setEnv: true,
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setEnv {
				t.Setenv(utils.EnvLocalHostNetworkConfigStatusDisable, tt.envVal)
			} else {
				os.Unsetenv(utils.EnvLocalHostNetworkConfigStatusDisable)
			}

			got := getDisableFromEnvOrDefault()
			if got != tt.want {
				t.Errorf("getDisableFromEnvOrDefault() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_getTTLFromEnvOrDefault(t *testing.T) {
	tests := []struct {
		name   string
		envVal string
		setEnv bool
		want   time.Duration
	}{
		{
			name:   "Env unset defaults to DefaultLocalHostNetworkConfigStatusTTL",
			setEnv: false,
			want:   utils.DefaultLocalHostNetworkConfigStatusTTL,
		},
		{
			name:   "Env empty defaults to DefaultLocalHostNetworkConfigStatusTTL",
			envVal: "",
			setEnv: true,
			want:   utils.DefaultLocalHostNetworkConfigStatusTTL,
		},
		{
			name:   "Env set to valid duration '10m'",
			envVal: "10m",
			setEnv: true,
			want:   10 * time.Minute,
		},
		{
			name:   "Env set to valid duration '30s'",
			envVal: "30s",
			setEnv: true,
			want:   30 * time.Second,
		},
		{
			name:   "Env set to zero '0s'",
			envVal: "0s",
			setEnv: true,
			want:   0,
		},
		{
			name:   "Env set to invalid duration string defaults to DefaultLocalHostNetworkConfigStatusTTL",
			envVal: "invalid-duration",
			setEnv: true,
			want:   utils.DefaultLocalHostNetworkConfigStatusTTL,
		},
		{
			name:   "Env set to negative duration defaults to DefaultLocalHostNetworkConfigStatusTTL",
			envVal: "-5m",
			setEnv: true,
			want:   utils.DefaultLocalHostNetworkConfigStatusTTL,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setEnv {
				t.Setenv(utils.EnvLocalHostNetworkConfigStatusTTL, tt.envVal)
			} else {
				os.Unsetenv(utils.EnvLocalHostNetworkConfigStatusTTL)
			}

			got := getTTLFromEnvOrDefault()
			if got != tt.want {
				t.Errorf("getTTLFromEnvOrDefault() = %v, want %v", got, tt.want)
			}
		})
	}
}
