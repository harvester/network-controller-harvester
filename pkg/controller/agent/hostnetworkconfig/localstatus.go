package hostnetworkconfig

import (
	"os"
	"sync"
	"time"

	"github.com/harvester/harvester-network-controller/pkg/utils"
	"github.com/sirupsen/logrus"
)

type ConfigState string

const (
	StateUnknown  ConfigState = ""         // Default zero-value (uninitialized or newly created)
	StateSetting  ConfigState = "Setting"  // Setup in progress on the node
	StateReady    ConfigState = "Ready"    // The node is involved in current HostNetworkConfig and setup is ready
	StateRemoving ConfigState = "Removing" // Cleanup/teardown in progress on the node
	StateRemoved  ConfigState = "Removed"  // The node is not involved in current HostNetworkConfig and cleanup is done
)

func (c ConfigState) String() string {
	if c == StateUnknown {
		return "Unknown"
	}
	return string(c)
}

// As HostNetworkConfig is a cluster-wide object that records the status of each node
// within a status map, any single node status change triggers a cluster-wide re-sync across nodes.
// This local cache object enables the agent controller on each node to determine whether it must perform
// operational work or quickly exit early (fast exit) to minimize unnecessary processing overhead.
type LocalHostNetworkConfigState struct {
	ConfigHash    string      // SHA-256 of the relevant Spec configuration
	Status        ConfigState // State: Initial, Setting, Ready, Cleaned
	LastValidated time.Time   // Timestamp when system status was last verified
}

// NewLocalHostNetworkConfigState creates a new state instance with LastValidated set to time.Now().
func NewLocalHostNetworkConfigState(configHash string, status ConfigState) LocalHostNetworkConfigState {
	return LocalHostNetworkConfigState{
		ConfigHash:    configHash,
		Status:        status,
		LastValidated: time.Now(),
	}
}

// isValidState encapsulates the common validation logic across different target states.
func (s LocalHostNetworkConfigState) isValidState(expectedStatus ConfigState, targetHash string, ttl time.Duration) bool {
	// empty targetHash is always invalid
	if targetHash == "" {
		return false
	}
	if s.Status != expectedStatus {
		return false
	}
	if s.ConfigHash != targetHash {
		return false
	}
	if ttl == 0 {
		return true
	}
	return time.Now().Before(s.LastValidated.Add(ttl))
}

// IsReady checks if the node configuration is ready, matches targetHash, and has not expired according to ttl.
func (s LocalHostNetworkConfigState) IsReady(targetHash string, ttl time.Duration) bool {
	return s.isValidState(StateReady, targetHash, ttl)
}

// IsRemoved checks if the node cleanup is completed, matches targetHash, and has not expired according to ttl.
func (s LocalHostNetworkConfigState) IsRemoved(targetHash string, ttl time.Duration) bool {
	return s.isValidState(StateRemoved, targetHash, ttl)
}

type LocalHostNetworkConfigStateManager struct {
	mu sync.RWMutex

	// ttl is set once during initialization and never modified.
	// The value originates from an environment variable passed to the pod runtime during initialization
	// and passed into NewLocalHostNetworkConfigStateManager.
	// If the environment variable changes, Kubernetes restarts/replaces the pod, creating a fresh manager.
	// Therefore, ttl is treated as strictly immutable during the process lifetime and is deliberately
	// not protected by the mutex for lock-free read performance.
	ttl time.Duration

	lhncs map[string]LocalHostNetworkConfigState
}

// NewLocalHostNetworkConfigStateManager creates a manager with a unified TTL for all node states.
// Set ttl to 0 for no expiration.
func NewLocalHostNetworkConfigStateManager(ttl time.Duration) *LocalHostNetworkConfigStateManager {
	return &LocalHostNetworkConfigStateManager{
		ttl:   ttl,
		lhncs: make(map[string]LocalHostNetworkConfigState),
	}
}

// TTL returns the configured TTL for the manager without mutex protection.
func (m *LocalHostNetworkConfigStateManager) TTL() time.Duration {
	return m.ttl
}

// Get returns a copy of the state for a given node.
func (m *LocalHostNetworkConfigStateManager) Get(nodeName string) (LocalHostNetworkConfigState, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	state, exists := m.lhncs[nodeName]
	return state, exists
}

// CreateOrUpdate accepts the node name and a LocalHostNetworkConfigState object.
// Returns true if a new entry was created, or false if an existing entry was updated.
func (m *LocalHostNetworkConfigStateManager) CreateOrUpdate(hnc string, state LocalHostNetworkConfigState) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, exists := m.lhncs[hnc]
	m.lhncs[hnc] = state
	return !exists
}

// Delete removes a node state entry.
func (m *LocalHostNetworkConfigStateManager) Delete(hnc string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.lhncs, hnc)
}

// getTTLFromEnvOrDefault returns the parsed time.Duration from the LOCAL_STATUS_TTL env var.
// If the variable is unset, empty, or fails to parse, it falls back to DefaultLocalStatusTTL.
func getTTLFromEnvOrDefault() time.Duration {
	envVal := os.Getenv(utils.EnvLocalHostNetworkConfigStatusTTL)
	if envVal == "" {
		return utils.DefaultLocalHostNetworkConfigStatusTTL
	}

	ttl, err := time.ParseDuration(envVal)
	if err != nil {
		logrus.Warnf("Failed to parse %s value %q, defaulting to %v: %v", utils.EnvLocalHostNetworkConfigStatusTTL, envVal, utils.DefaultLocalHostNetworkConfigStatusTTL, err)
		return utils.DefaultLocalHostNetworkConfigStatusTTL
	}

	return ttl
}
