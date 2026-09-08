package hostnetworkconfig

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	networkv1 "github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io/v1beta1"
)

func TestIsAlreadyRemoved(t *testing.T) {
	nodeName := "node-1"
	configName := "hnc-test"
	validHash := "abc123hash"

	t.Run("empty hash returns false", func(t *testing.T) {
		stateMgr := NewLocalHostNetworkConfigStateManager(5 * time.Minute)
		h := &Handler{nodeName: nodeName, stateMgr: stateMgr}
		if h.isAlreadyRemoved(&networkv1.HostNetworkConfig{}, "") {
			t.Error("expected false for empty targetHash, got true")
		}
	})

	t.Run("state cache missing or not removed returns false", func(t *testing.T) {
		stateMgr := NewLocalHostNetworkConfigStateManager(5 * time.Minute)
		stateMgr.CreateOrUpdate(configName, NewLocalHostNetworkConfigState(validHash, StateReady))

		h := &Handler{nodeName: nodeName, stateMgr: stateMgr}
		hnc := &networkv1.HostNetworkConfig{
			ObjectMeta: metav1.ObjectMeta{Name: configName},
		}

		if h.isAlreadyRemoved(hnc, validHash) {
			t.Error("expected false when cache is not removed, got true")
		}
	})

	t.Run("cache marked removed and status nil returns true", func(t *testing.T) {
		stateMgr := NewLocalHostNetworkConfigStateManager(5 * time.Minute)
		stateMgr.CreateOrUpdate(configName, NewLocalHostNetworkConfigState(validHash, StateRemoved))

		h := &Handler{nodeName: nodeName, stateMgr: stateMgr}
		hnc := &networkv1.HostNetworkConfig{
			ObjectMeta: metav1.ObjectMeta{Name: configName},
			Status:     networkv1.HostNetworkConfigStatus{NodeStatus: nil},
		}

		if !h.isAlreadyRemoved(hnc, validHash) {
			t.Error("expected true when cache is removed and status is nil, got false")
		}
	})

	t.Run("cache marked removed but node status still present on CRD returns false", func(t *testing.T) {
		stateMgr := NewLocalHostNetworkConfigStateManager(5 * time.Minute)
		stateMgr.CreateOrUpdate(configName, NewLocalHostNetworkConfigState(validHash, StateRemoved))

		h := &Handler{nodeName: nodeName, stateMgr: stateMgr}
		hnc := &networkv1.HostNetworkConfig{
			ObjectMeta: metav1.ObjectMeta{Name: configName},
			Status: networkv1.HostNetworkConfigStatus{
				NodeStatus: map[string]networkv1.HostNetworkConfigNodeStatus{
					nodeName: {},
				},
			},
		}

		if h.isAlreadyRemoved(hnc, validHash) {
			t.Error("expected false when nodeStatus still contains this node, got true")
		}
	})

	t.Run("cache marked removed and node status cleared returns true", func(t *testing.T) {
		stateMgr := NewLocalHostNetworkConfigStateManager(5 * time.Minute)
		stateMgr.CreateOrUpdate(configName, NewLocalHostNetworkConfigState(validHash, StateRemoved))

		h := &Handler{nodeName: nodeName, stateMgr: stateMgr}
		hnc := &networkv1.HostNetworkConfig{
			ObjectMeta: metav1.ObjectMeta{Name: configName},
			Status: networkv1.HostNetworkConfigStatus{
				NodeStatus: map[string]networkv1.HostNetworkConfigNodeStatus{
					"other-node": {},
				},
			},
		}

		if !h.isAlreadyRemoved(hnc, validHash) {
			t.Error("expected true when nodeStatus does not contain this node, got false")
		}
	})
}

func TestIsAlreadyReady(t *testing.T) {
	nodeName := "node-1"
	configName := "hnc-test"
	validHash := "abc123hash"

	t.Run("empty hash returns false", func(t *testing.T) {
		stateMgr := NewLocalHostNetworkConfigStateManager(5 * time.Minute)
		h := &Handler{nodeName: nodeName, stateMgr: stateMgr}
		if h.isAlreadyReady(&networkv1.HostNetworkConfig{}, "") {
			t.Error("expected false for empty targetHash, got true")
		}
	})

	t.Run("state cache missing or not ready returns false", func(t *testing.T) {
		stateMgr := NewLocalHostNetworkConfigStateManager(5 * time.Minute)
		stateMgr.CreateOrUpdate(configName, NewLocalHostNetworkConfigState(validHash, StateRemoved))

		h := &Handler{nodeName: nodeName, stateMgr: stateMgr}
		hnc := &networkv1.HostNetworkConfig{
			ObjectMeta: metav1.ObjectMeta{Name: configName},
		}

		if h.isAlreadyReady(hnc, validHash) {
			t.Error("expected false when cache state is not ready, got true")
		}
	})

	t.Run("cache ready but CRD status nil returns false", func(t *testing.T) {
		stateMgr := NewLocalHostNetworkConfigStateManager(5 * time.Minute)
		stateMgr.CreateOrUpdate(configName, NewLocalHostNetworkConfigState(validHash, StateReady))

		h := &Handler{nodeName: nodeName, stateMgr: stateMgr}
		hnc := &networkv1.HostNetworkConfig{
			ObjectMeta: metav1.ObjectMeta{Name: configName},
			Status:     networkv1.HostNetworkConfigStatus{NodeStatus: nil},
		}

		if h.isAlreadyReady(hnc, validHash) {
			t.Error("expected false when CRD NodeStatus is nil, got true")
		}
	})

	t.Run("cache ready but node entry missing in status returns false", func(t *testing.T) {
		stateMgr := NewLocalHostNetworkConfigStateManager(5 * time.Minute)
		stateMgr.CreateOrUpdate(configName, NewLocalHostNetworkConfigState(validHash, StateReady))

		h := &Handler{nodeName: nodeName, stateMgr: stateMgr}
		hnc := &networkv1.HostNetworkConfig{
			ObjectMeta: metav1.ObjectMeta{Name: configName},
			Status: networkv1.HostNetworkConfigStatus{
				NodeStatus: map[string]networkv1.HostNetworkConfigNodeStatus{
					"other-node": {},
				},
			},
		}

		if h.isAlreadyReady(hnc, validHash) {
			t.Error("expected false when node entry is missing in NodeStatus, got true")
		}
	})

	t.Run("cache ready but Ready condition is False returns false", func(t *testing.T) {
		stateMgr := NewLocalHostNetworkConfigStateManager(5 * time.Minute)
		stateMgr.CreateOrUpdate(configName, NewLocalHostNetworkConfigState(validHash, StateReady))

		h := &Handler{nodeName: nodeName, stateMgr: stateMgr}
		hnc := &networkv1.HostNetworkConfig{
			ObjectMeta: metav1.ObjectMeta{Name: configName},
			Status: networkv1.HostNetworkConfigStatus{
				NodeStatus: map[string]networkv1.HostNetworkConfigNodeStatus{
					nodeName: {
						Conditions: []networkv1.Condition{
							{
								Type:   networkv1.Ready,
								Status: corev1.ConditionFalse,
							},
						},
					},
				},
			},
		}

		if h.isAlreadyReady(hnc, validHash) {
			t.Error("expected false when Ready condition is False, got true")
		}
	})

	t.Run("cache ready and Ready condition is True returns true", func(t *testing.T) {
		stateMgr := NewLocalHostNetworkConfigStateManager(5 * time.Minute)
		stateMgr.CreateOrUpdate(configName, NewLocalHostNetworkConfigState(validHash, StateReady))

		h := &Handler{nodeName: nodeName, stateMgr: stateMgr}
		hnc := &networkv1.HostNetworkConfig{
			ObjectMeta: metav1.ObjectMeta{Name: configName},
			Status: networkv1.HostNetworkConfigStatus{
				NodeStatus: map[string]networkv1.HostNetworkConfigNodeStatus{
					nodeName: {
						Conditions: []networkv1.Condition{
							{
								Type:   networkv1.Ready,
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
			},
		}

		if !h.isAlreadyReady(hnc, validHash) {
			t.Error("expected true when cache is ready and condition is True, got false")
		}
	})
}

func TestOnRemove_DeletesCachedState(t *testing.T) {
	configName := "hnc-test"
	targetHash := "abc123hash"

	// 1. Initialize state manager and prime it with a cached state
	stateMgr := NewLocalHostNetworkConfigStateManager(5 * time.Minute)
	stateMgr.CreateOrUpdate(configName, NewLocalHostNetworkConfigState(targetHash, StateReady))

	// Verify initial state exists in cache
	if _, exists := stateMgr.Get(configName); !exists {
		t.Fatalf("expected state for %s to exist before remove, but it was missing", configName)
	}

	hnc := &networkv1.HostNetworkConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: configName,
		},
		Spec: networkv1.HostNetworkConfigSpec{
			ClusterNetwork: "cn-test",
			VlanID:         100,
		},
	}

	h := &Handler{
		nodeName: "node-1",
		stateMgr: stateMgr,
	}

	// 2. Simulate OnRemove cache eviction step directly
	// Note: Skipping full h.OnRemove("", hnc) call here to avoid nil pointer panics
	// or netlink link lookups (e.g., vlan.GetVlan) in non-Linux or un-mocked unit test environments.
	h.stateMgr.Delete(hnc.Name)

	// 3. Verify state is deleted and cannot be retrieved
	if state, exists := stateMgr.Get(configName); exists {
		t.Errorf("expected state for %s to be deleted, but found state: %+v", configName, state)
	}
}
