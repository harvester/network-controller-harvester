package bridge

import (
	"fmt"
	"time"

	"github.com/vishvananda/netlink"
	"k8s.io/klog"

	"github.com/rancher/harvester-network-controller/pkg/dhcp"
)

const defaultPVID = uint16(1)

type Link struct {
	netlink.Link
}

// startDHCP/stopDHCP only support for physical nic
func (l *Link) startDHCP() error {
	return dhcp.NewConnmanService(l.Attrs().Name, l.Attrs().HardwareAddr.String()).ToDHCP()
}

// stopDHCP transfer DHCP to static mode and delete ip route to make it not work
// If we set off mode, the physical nic, also used by flannel, will down without ip address,
// resulting in that k3s can't start after restarting node.
func (l *Link) stopDHCP() error {
	cs := dhcp.NewConnmanService(l.Attrs().Name, l.Attrs().HardwareAddr.String())
	mode, err := cs.GetIPv4Mode()
	if err != nil {
		return fmt.Errorf("get ipv4 mode failed, error: %w, iface: %s", err, l.Attrs().Name)
	}

	if mode == dhcp.DHCP {
		if err := dhcp.NewConnmanService(l.Attrs().Name, l.Attrs().HardwareAddr.String()).DHCP2Static(); err != nil {
			return err
		}
	}

	return nil
}

// GetLink by name
func GetLink(name string) (*Link, error) {
	if name == "" {
		return nil, nil
	}

	l, err := netlink.LinkByName(name)
	if err != nil {
		return nil, fmt.Errorf("could not lookup link, error: %w, link: %s", err, name)
	}

	return &Link{Link: l}, nil
}

// AddBridgeVlan adds a new vlan filter entry
// Equivalent to: `bridge vlan add dev DEV vid VID master`
func (l *Link) AddBridgeVlan(vid uint16) error {
	if vid == defaultPVID {
		return nil
	}

	if err := netlink.BridgeVlanAdd(l.Link, vid, false, false, false, true); err != nil {
		return fmt.Errorf("add bridge vlan failed, error: %v, link: %s, vid: %d", err, l.Attrs().Name, vid)
	}

	return nil
}

// DelBridgeVlan adds a new vlan filter entry
// Equivalent to: `bridge vlan del dev DEV vid VID master`
func (l *Link) DelBridgeVlan(vid uint16) error {
	if vid == defaultPVID {
		return nil
	}

	if err := netlink.BridgeVlanDel(l.Link, vid, false, false, false, true); err != nil {
		return fmt.Errorf("delete bridge vlan failed, error: %v, link: %s, vid: %d", err, l.Attrs().Name, vid)
	}

	return nil
}

const (
	defaultRetryTimes    = 10
	defaultRetryInterval = time.Second
)

func (l *Link) setNoMaster() error {
	if err := netlink.LinkSetNoMaster(l); err != nil {
		return fmt.Errorf("could not set no master, error: %w, link: %s", err, l.Attrs().Name)
	}

	// link will be down after a while
	for i := 0; i < defaultRetryTimes; i++ {
		klog.Infof("check link down, retry times: %d", i+1)
		l, err := netlink.LinkByName(l.Attrs().Name)
		if err != nil {
			return fmt.Errorf("get link %s failed, error: %w", l.Attrs().Name, err)
		}

		if l.Attrs().OperState == netlink.OperDown {
			if err := netlink.LinkSetUp(l); err != nil {
				return fmt.Errorf("could not set up %s, error: %w", l.Attrs().Name, err)
			}
			break
		}

		time.Sleep(defaultRetryInterval)
	}

	return nil
}
