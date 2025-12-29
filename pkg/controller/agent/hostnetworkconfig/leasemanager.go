package hostnetworkconfig

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/harvester/harvester-network-controller/pkg/network/iface"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/insomniacslk/dhcp/dhcpv4/nclient4"
)

type LeaseManager struct {
	iface  string
	link   *iface.Link
	vlanID uint16
	client *nclient4.Client

	mu      sync.Mutex
	lease   *nclient4.Lease
	ipAddr  string
	running bool

	ctx    context.Context
	cancel context.CancelFunc
}

func NewLeaseManager(iface string, link *iface.Link, vlanID uint16) (*LeaseManager, error) {
	c, err := nclient4.New(iface)
	if err != nil {
		return nil, err
	}

	return &LeaseManager{
		iface:  iface,
		link:   link,
		vlanID: vlanID,
		client: c,
	}, nil
}

func (lm *LeaseManager) Start(ctx context.Context) error {
	lm.mu.Lock()
	if lm.running {
		lm.mu.Unlock()
		return nil
	}
	lm.mu.Unlock()

	lm.ctx, lm.cancel = context.WithCancel(ctx)

	lease, err := lm.client.Request(lm.ctx)
	if err != nil {
		return err
	}

	ipAddr, err := ipAddrFromLease(lease)
	if err != nil {
		return err
	}

	if ipAddr == "" {
		return fmt.Errorf("no IP address obtained from DHCP server")
	}

	if err := lm.link.SetIPAddress(ipAddr, lm.vlanID); err != nil {
		return err
	}

	lm.mu.Lock()
	lm.lease = lease
	lm.ipAddr = ipAddr
	lm.running = true
	lm.mu.Unlock()

	go lm.renewLoop()

	return nil
}

func renewalDelay(lease *dhcpv4.DHCPv4) time.Duration {
	// T1 (option 58), default fallback = 50% of lease
	if t1 := lease.IPAddressRenewalTime(0); t1 > 0 {
		return t1
	}

	// Fallback: 50% of lease time
	if lt := lease.IPAddressLeaseTime(0); lt > 0 {
		return lt / 2
	}

	// Absolute fallback
	return 30 * time.Second
}

func (lm *LeaseManager) renewLoop() {
	for {
		lm.mu.Lock()
		lease := lm.lease
		lm.mu.Unlock()

		if lease == nil {
			return
		}

		delay := renewalDelay(lease.ACK)
		timer := time.NewTimer(delay)

		select {
		case <-timer.C:
			newLease, err := lm.client.Renew(lm.ctx, lease)
			if err != nil {
				newLease, err = lm.client.Request(lm.ctx)
				if err != nil {
					continue
				}
			}

			ipAddr, err := ipAddrFromLease(newLease)
			if err != nil {
				continue
			}

			lm.mu.Lock()
			lm.lease = newLease
			sameIP := ipAddr == lm.ipAddr
			lm.mu.Unlock()

			if sameIP {
				continue
			}

			if err := lm.link.SetIPAddress(ipAddr, lm.vlanID); err != nil {
				continue
			}

			lm.mu.Lock()
			lm.ipAddr = ipAddr
			lm.mu.Unlock()

		case <-lm.ctx.Done():
			timer.Stop()
			return
		}
	}
}

func (lm *LeaseManager) Stop() {
	if lm == nil {
		return
	}

	lm.mu.Lock()
	if !lm.running {
		lm.mu.Unlock()
		return
	}

	cancel := lm.cancel
	lease := lm.lease

	lm.running = false
	lm.cancel = nil
	lm.lease = nil
	lm.mu.Unlock()

	if cancel != nil {
		cancel()
	}

	// Release DHCP lease
	if lease != nil {
		_ = lm.client.Release(lease)
	}
}

func ipAddrFromLease(lease *nclient4.Lease) (string, error) {
	maskOpt := lease.ACK.Options.Get(dhcpv4.OptionSubnetMask)
	if maskOpt == nil {
		return "", fmt.Errorf("subnet mask not provided by DHCP server")
	}

	// DHCPv4 subnet mask option is exactly 4 bytes
	if len(maskOpt) != 4 {
		return "", fmt.Errorf("invalid subnet mask length: %d", len(maskOpt))
	}

	// Convert to net.IPMask
	ipMask := net.IPMask(maskOpt)

	ones, bits := ipMask.Size()
	if bits != 32 {
		return "", fmt.Errorf("invalid IPv4 subnet mask")
	}

	ipAddr := fmt.Sprintf("%s/%d", lease.ACK.YourIPAddr.String(), ones)

	return ipAddr, nil
}
