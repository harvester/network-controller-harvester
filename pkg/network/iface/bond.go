package iface

import (
	"errors"
	"fmt"
	"net"

	"github.com/vishvananda/netlink"
)

const BondSuffix = "-bo"

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

// EnsureBond cares about the bond attributes excluding the master index and the slaves
func (b *Bond) EnsureBond() error {
	if err := b.ensureBond(); err != nil {
		return err
	}

	return b.ensureBondSlaves()
}

func (b *Bond) ensureBond() error {
	// add or update
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
	// add slaves
	for _, slave := range b.slaves {
		l := slaveMap[slave]
		if l == nil {
			l, err = netlink.LinkByName(slave)
			if err != nil {
				return fmt.Errorf("get link %s failed, error: %w", slave, err)
			}
			// return error if the link has been enslaved
			if l.Attrs().MasterIndex != 0 && l.Attrs().MasterIndex != b.Index {
				return fmt.Errorf("%s has been enslaved by the link with index %d", l.Attrs().Name, l.Attrs().MasterIndex)
			}
			// The slave link should be down before enslaved, otherwise, there will be error like `operation not permitted`.
			if err := netlink.LinkSetDown(l); err != nil {
				return fmt.Errorf("set slave %s down failed, error: %w", slave, err)
			}
			if err := netlink.LinkSetBondSlave(l, b.Bond); err != nil {
				return fmt.Errorf("add slave %s to bond %s failed, error: %w", slave, b.Name, err)
			}
		}

		if l.Attrs().Flags&net.FlagUp == 0 {
			if err := netlink.LinkSetUp(l); err != nil {
				return err
			}
		}

		// delete the handled slave
		delete(slaveMap, slave)
	}
	// delete slaves which still remain in the map
	for name, l := range slaveMap {
		if err := netlink.LinkSetNoMaster(l); err != nil {
			return fmt.Errorf("delete slave %s from %s failed, error: %w", name, b.Name, err)
		}
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

	for _, slave := range slaves {
		if err := netlink.LinkSetUp(slave); err != nil {
			return err
		}
	}

	return nil
}

// delete the original bond and create new one
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
