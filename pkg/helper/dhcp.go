package helper

import (
	"context"
	"fmt"
	"net"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/insomniacslk/dhcp/dhcpv4/nclient4"
	"github.com/vishvananda/netlink"
)

func obtainCIDRAndGw(iface string, serverAddr net.IP) (*net.IPNet, net.IP, error) {
	var ack *dhcpv4.DHCPv4
	var err error
	if serverAddr != nil {
		ack, err = sendInformMessage(iface, serverAddr)
	} else {
		ack, err = sendDiscoverMessage(iface)
	}

	if err != nil {
		return nil, nil, err
	}

	defaultGateway := net.IP(ack.Options.Get(dhcpv4.OptionRouter))
	cidr := &net.IPNet{}
	cidr.Mask = ack.Options.Get(dhcpv4.OptionSubnetMask)
	cidr.IP = defaultGateway.Mask(cidr.Mask)

	return cidr, defaultGateway, nil
}

func sendDiscoverMessage(iface string) (*dhcpv4.DHCPv4, error) {
	broadcast, err := nclient4.New(iface)
	if err != nil {
		return nil, err
	}
	defer broadcast.Close()
	return broadcast.DiscoverOffer(context.TODO())
}

func sendInformMessage(iface string, siaddr net.IP) (*dhcpv4.DHCPv4, error) {
	l, err := netlink.LinkByName(iface)
	if err != nil {
		return nil, err
	}
	addresses, err := netlink.AddrList(l, netlink.FAMILY_V4)
	if err != nil {
		return nil, err
	}
	if len(addresses) < 1 {
		return nil, fmt.Errorf("not found IP address")
	}

	ciaddr := addresses[0].IP
	hwaddr := l.Attrs().HardwareAddr

	unicast, err := nclient4.New(iface, nclient4.WithUnicast(&net.UDPAddr{Port: dhcpv4.ClientPort}),
		nclient4.WithServerAddr(&net.UDPAddr{IP: siaddr, Port: nclient4.ServerPort}))
	if err != nil {
		return nil, err
	}
	defer unicast.Close()

	inform, err := newInform(ciaddr, hwaddr)
	if err != nil {
		return nil, err
	}

	response, err := unicast.SendAndRead(context.TODO(), unicast.RemoteAddr(), inform, nil)
	if err != nil {
		return nil, err
	}

	if response.MessageType() == dhcpv4.MessageTypeNak {
		return nil, fmt.Errorf("receive nak, %+v", response)
	}

	return response, nil
}

func newInform(ciaddr net.IP, hwaddr net.HardwareAddr) (*dhcpv4.DHCPv4, error) {
	return dhcpv4.New(dhcpv4.WithHwAddr(hwaddr),
		dhcpv4.WithClientIP(ciaddr),
		dhcpv4.WithRequestedOptions(
			dhcpv4.OptionSubnetMask,
			dhcpv4.OptionRouter,
			dhcpv4.OptionDomainName,
			dhcpv4.OptionDomainNameServer,
		),
		dhcpv4.WithMessageType(dhcpv4.MessageTypeInform))
}
