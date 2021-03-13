package iface

import "github.com/vishvananda/netlink"

type Nic struct {
	Link                netlink.Link
	UsedByManageNetwork bool
}

const (
	flannelName = "flannel.1"

	typeLoopback = "loopback"
	typeVxlan    = "vxlan"
)

func GetPhysicalNICs() (map[int]*Nic, error) {
	nics := map[int]*Nic{}
	links, err := netlink.LinkList()
	if err != nil {
		return nil, err
	}

	for _, link := range links {
		if link.Type() == "device" && link.Attrs().EncapType != typeLoopback {
			nics[link.Attrs().Index] = &Nic{Link: link}
		}
	}

	for _, link := range links {
		if link.Type() == typeVxlan && link.Attrs().Name == flannelName {
			n, ok := nics[link.(*netlink.Vxlan).VtepDevIndex]
			if ok {
				n.UsedByManageNetwork = true
			}
		}
	}

	return nics, nil
}
