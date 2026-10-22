package iface

import (
	"fmt"

	"github.com/containernetworking/plugins/pkg/netlinksafe"
	"github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"
)

// UplinkAttrs are the link attributes applied to every NIC of an isolated-mode uplink
type UplinkAttrs struct {
	MTU    int
	TxQLen int
}

// ListBridgePorts lists the links of the given type whose master is the bridge with index brIndex
func ListBridgePorts(brIndex int, linkType string) ([]*Link, error) {
	links, err := netlinksafe.LinkList()
	if err != nil {
		return nil, err
	}

	var ports []*Link
	for _, l := range links {
		if l.Attrs().MasterIndex == brIndex && l.Type() == linkType {
			ports = append(ports, NewLink(l))
		}
	}

	return ports, nil
}

// EnsureIsolatedPort attaches the NIC to the bridge as an isolated bridge port and applies the link attributes.
// A NIC that is not attached yet is set down, attached and isolated before it is brought up, so it never forwards
// to another uplink. Like the slaves of a bond uplink, the NIC is always brought up at the end, which also recovers
// a NIC left down by an earlier, partially failed attach.
func EnsureIsolatedPort(name string, br *Bridge, attrs *UplinkAttrs) (*Link, error) {
	l, err := netlink.LinkByName(name)
	if err != nil {
		return nil, fmt.Errorf("get link %s failed, error: %w", name, err)
	}

	master := l.Attrs().MasterIndex
	if master != 0 && master != br.Index {
		return nil, fmt.Errorf("%s has been enslaved by the link with index %d", name, master)
	}

	attach := master == 0
	if attach {
		if err := netlink.LinkSetDown(l); err != nil {
			return nil, fmt.Errorf("set %s down failed, error: %w", name, err)
		}
	}

	if attrs != nil {
		if attrs.MTU > 0 && l.Attrs().MTU != attrs.MTU {
			if err := netlink.LinkSetMTU(l, attrs.MTU); err != nil {
				return nil, fmt.Errorf("set %s mtu %d failed, error: %w", name, attrs.MTU, err)
			}
		}
		if attrs.TxQLen >= 0 && l.Attrs().TxQLen != attrs.TxQLen {
			if err := netlink.LinkSetTxQLen(l, attrs.TxQLen); err != nil {
				return nil, fmt.Errorf("set %s txqlen %d failed, error: %w", name, attrs.TxQLen, err)
			}
		}
	}

	link := NewLink(l)
	if attach {
		if err := link.SetMaster(br); err != nil {
			return nil, err
		}
		logrus.Infof("attached %s to %s as an isolated port", name, br.Name)
	}

	if err := ensureIsolated(l); err != nil {
		return nil, err
	}

	if err := setLinkUp(name); err != nil {
		return nil, err
	}

	if err := link.Fetch(); err != nil {
		return nil, err
	}

	return link, nil
}

func ensureIsolated(l netlink.Link) error {
	pi, err := netlink.LinkGetProtinfo(l)
	if err == nil && pi.Isolated {
		return nil
	}
	if err := netlink.LinkSetIsolated(l, true); err != nil {
		return fmt.Errorf("set %s isolated failed, error: %w", l.Attrs().Name, err)
	}
	return nil
}
