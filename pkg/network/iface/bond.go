package iface

import (
	"errors"
	"fmt"
	"net"

	"github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"
)

type Bond struct {
	*netlink.Bond
	slaves []string
}

func NewBond(bond *netlink.Bond, slaves []string) *Bond {
	return &Bond{
		Bond:   bond,
		slaves: slaves,
	}
}

// setLinkUp safely sets a network link to UP state with proper checks and logging
func setLinkUp(link netlink.Link) error {
	ifName := link.Attrs().Name
	currentState := link.Attrs().OperState
	currentFlags := link.Attrs().Flags

	// Check if already UP
	if currentState == netlink.OperUp || (currentFlags&net.FlagUp) != 0 {
		logrus.Debugf("NIC %s is already UP, skipping", ifName)
		return nil
	}

	logrus.Infof("Setting NIC %s to UP state (current: %s)", ifName, currentState)
	if err := netlink.LinkSetUp(link); err != nil {
		return fmt.Errorf("failed to set NIC %s up: %w", ifName, err)
	}

	logrus.Infof("NIC %s successfully set to UP state", ifName)
	return nil
}

// EnsureBond cares about the bond attributes excluding the master index and the slaves
func (b *Bond) EnsureBond() error {
	if err := b.ensureBond(); err != nil {
		return err
	}

	return b.ensureBondSlaves()
}

func (b *Bond) ensureBond() error {
	// add or update bond
	if oldBond, err := netlink.LinkByName(b.Name); errors.As(err, &netlink.LinkNotFoundError{}) {
		if err := netlink.LinkAdd(b.Bond); err != nil {
			return fmt.Errorf("add bond %s failed, error: %w", b.Name, err)
		}
	} else if err != nil {
		return fmt.Errorf("get bond %s failed, error: %w", b.Name, err)
	} else {
		if err := b.modifyBond(oldBond.(*netlink.Bond)); err != nil {
			return fmt.Errorf("modify bond %s failed, error: %w", b.Name, err)
		}
	}
	if err := netlink.LinkSetUp(b); err != nil {
		return fmt.Errorf("set %s up failed, error: %w", b.Name, err)
	}
	// fetch bond
	l, err := netlink.LinkByName(b.Name)
	if err != nil {
		return fmt.Errorf("fetch bond %s failed, error: %w", b.Name, err)
	}
	bond, ok := l.(*netlink.Bond)
	if !ok {
		return fmt.Errorf("%s already exists but is not a bond", bond.Name)
	}
	b.Bond = bond

	return nil
}

func (b *Bond) ensureBondSlaves() error {
	links, err := getSlaves(b.Index)
	if err != nil {
		return err
	}
	slaveMap := make(map[string]netlink.Link)
	for _, l := range links {
		slaveMap[l.Attrs().Name] = l
	}

	// add slaves to bond
	for _, slave := range b.slaves {
		l := slaveMap[slave]
		if l == nil {
			l, err = netlink.LinkByName(slave)
			if err != nil {
				return fmt.Errorf("get link %s failed, error: %w", slave, err)
			}
			// return error if the link has been enslaved by another master
			if l.Attrs().MasterIndex != 0 && l.Attrs().MasterIndex != b.Index {
				return fmt.Errorf("%s has been enslaved by the link with index %d", l.Attrs().Name, l.Attrs().MasterIndex)
			}

			// The slave link should be down before enslaved
			if err := netlink.LinkSetDown(l); err != nil {
				return fmt.Errorf("set slave %s down failed, error: %w", slave, err)
			}

			if err := netlink.LinkSetBondSlave(l, b.Bond); err != nil {
				if upErr := setLinkUp(l); upErr != nil {
					logrus.Warnf("Failed to set NIC %s up after bond operation failed: %v", slave, upErr)
				}
				return fmt.Errorf("add slave %s to bond %s failed, error: %w", slave, b.Name, err)
			}
		}

		// Ensure slave is in UP state after being added to bond
		if l.Attrs().Flags&net.FlagUp == 0 {
			if err := setLinkUp(l); err != nil {
				return err
			}
		}

		// Remove the handled slave from the map
		delete(slaveMap, slave)
	}

	// Remove slaves that are no longer in the desired configuration
	// Collect all removal errors to provide comprehensive status
	var removalErrors []error
	for name, l := range slaveMap {
		// First remove from bond
		if err := netlink.LinkSetNoMaster(l); err != nil {
			removalErrors = append(removalErrors,
				fmt.Errorf("delete slave %s from %s failed: %w", name, b.Name, err))
			continue
		}

		if err := setLinkUp(l); err != nil {
			removalErrors = append(removalErrors,
				fmt.Errorf("set NIC %s up after removal failed: %w", name, err))
		} else {
			logrus.Infof("NIC %s removed from bond %s and set to UP state", name, b.Name)
		}
	}

	// If any removal operations failed, return aggregated error
	if len(removalErrors) > 0 {
		return fmt.Errorf("failed to clean up %d slave(s): %v", len(removalErrors), removalErrors)
	}

	return nil
}

func (b *Bond) remove() error {
	slaves, err := getSlaves(b.Index)
	if err != nil {
		return err
	}

	if err := netlink.LinkDel(b); err != nil {
		return err
	}

	// Collect all slave recovery errors
	var recoveryErrors []error
	for _, slave := range slaves {
		// Ensure slave is in UP state after bond deletion
		if err := setLinkUp(slave); err != nil {
			recoveryErrors = append(recoveryErrors,
				fmt.Errorf("failed to set NIC %s up after bond deletion: %w", slave.Attrs().Name, err))
		}
	}

	// Return aggregated error if any slave recovery failed
	if len(recoveryErrors) > 0 {
		return fmt.Errorf("bond deletion incomplete: %d NIC(s) failed to recover: %v",
			len(recoveryErrors), recoveryErrors)
	}

	return nil
}

// modifyBond deletes the original bond and creates a new one
func (b *Bond) modifyBond(oldBond *netlink.Bond) error {
	if compareBond(oldBond, b.Bond) {
		return nil
	}

	if err := netlink.LinkDel(oldBond); err != nil {
		return err
	}
	return netlink.LinkAdd(b.Bond)
}

func getSlaves(index int) ([]netlink.Link, error) {
	if index == 0 {
		return nil, fmt.Errorf("invalid master index %d", index)
	}

	all, err := netlink.LinkList()
	if err != nil {
		return nil, fmt.Errorf("list links failed, error: %w", err)
	}

	links := make([]netlink.Link, 0, len(all))
	for _, l := range all {
		if l.Attrs().MasterIndex == index {
			links = append(links, l)
		}
	}

	return links, nil
}

func compareBond(old, new *netlink.Bond) bool { //nolint
	if old.Name != new.Name {
		return false
	}

	if old.Mode != new.Mode {
		return false
	}

	// skip if mtu is omitted
	if new.MTU != 0 && old.MTU != new.MTU {
		return false
	}

	// skip if hardware addr is omitted
	if new.HardwareAddr.String() != "" && old.HardwareAddr.String() != new.HardwareAddr.String() {
		return false
	}

	// skip if TxQLen is omitted, default value -1
	// -1 means to keep the original TxQLen value, so we have to skip it to avoid unnecessary bond recreating.
	if new.TxQLen != -1 && old.TxQLen != new.TxQLen {
		return false
	}

	// skip if Miimon is omitted, default value -1
	// Same logic with TxQLen
	if new.Miimon != -1 && old.Miimon != new.Miimon {
		return false
	}

	return true
}
