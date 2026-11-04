package hostnetworkconfig

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	networkv1 "github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io/v1beta1"

	"github.com/harvester/harvester-network-controller/pkg/network/iface"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/insomniacslk/dhcp/dhcpv4/nclient4"
	"github.com/insomniacslk/dhcp/dhcpv6"
	"github.com/insomniacslk/dhcp/dhcpv6/nclient6"
)

type LeaseManager struct {
	iface  string
	link   *iface.Link
	vlanID uint16
	family networkv1.IPFamily

	client4 *nclient4.Client
	client6 *nclient6.Client

	mu     sync.RWMutex
	lease4 *nclient4.Lease
	lease6 dhcpv6.DHCPv6
	// netState explicitly books and records the active DHCP lease info
	// (such as assigned IP addresses and custom server-offered gateways).
	netState networkv1.NodeNetworkState
	running  bool

	ctx    context.Context
	cancel context.CancelFunc
}

func NewLeaseManager(iface string, link *iface.Link, vlanID uint16, family networkv1.IPFamily) (*LeaseManager, error) {
	if family == "" {
		family = networkv1.IPFamilyIPv4 // Default fallback
	}

	lm := &LeaseManager{
		iface:  iface,
		link:   link,
		vlanID: vlanID,
		family: family,
	}

	var err error
	switch family {
	case networkv1.IPFamilyIPv4:
		lm.client4, err = nclient4.New(iface)
		if err != nil {
			return nil, fmt.Errorf("failed to create DHCPv4 client: %w", err)
		}
	case networkv1.IPFamilyIPv6:
		// 1. Brief pause to allow the kernel to assign the IPv6 link-local address on the newly created interface
		time.Sleep(200 * time.Millisecond)
		lm.client6, err = nclient6.New(iface)
		if err != nil {
			return nil, fmt.Errorf("failed to create DHCPv6 client: %w", err)
		}
	case networkv1.IPFamilyDualIPv4First:
		lm.client4, err = nclient4.New(iface)
		if err != nil {
			return nil, fmt.Errorf("failed to create DHCPv4 client: %w", err)
		}
		// 1. Brief pause to allow the kernel to assign the IPv6 link-local address on the newly created interface
		time.Sleep(200 * time.Millisecond)
		lm.client6, err = nclient6.New(iface)
		if err != nil {
			// close the above ready client4
			_ = lm.client4.Close()
			return nil, fmt.Errorf("failed to create DHCPv6 client: %w", err)
		}
	}

	return lm, nil
}

func (lm *LeaseManager) Start(ctx context.Context) (*networkv1.NodeNetworkState, error) {
	// Use RLock for the read-only check of running status and existing netState
	lm.mu.RLock()
	if lm.running {
		state := lm.netState
		lm.mu.RUnlock()
		return &state, nil
	}
	lm.mu.RUnlock()

	lm.mu.Lock()
	lm.ctx, lm.cancel = context.WithCancel(ctx)
	lm.mu.Unlock()
	// Ensure cleanup if Start fails midway before setting running = true
	var success bool
	defer func() {
		if !success {
			lm.Stop()
		}
	}()

	type ipTask struct {
		isV4    bool
		reqFunc func() (ipAddr string, gateway *string, err error)
	}

	v4Task := ipTask{
		isV4: true,
		reqFunc: func() (string, *string, error) {
			lease4, err := lm.client4.Request(lm.ctx)
			if err != nil {
				return "", nil, fmt.Errorf("failed to get IPv4 lease: %w", err)
			}
			ip4, err := ipAddrFromLeaseV4(lease4)
			if err != nil {
				// If parsing/validation fails immediately after acquiring the lease,
				// we use the original client to release it since the IP is not yet
				// bound locally to the network interface. The best effort we do.
				_ = lm.client4.Release(lease4)
				return "", nil, err
			}

			var gwPtr *string
			if routers := lease4.ACK.Router(); len(routers) > 0 && routers[0] != nil && !routers[0].IsUnspecified() {
				gw := routers[0].String()
				gwPtr = &gw
			}

			lm.mu.Lock()
			lm.lease4 = lease4
			lm.mu.Unlock()
			return ip4, gwPtr, nil
		},
	}

	v6Task := ipTask{
		isV4: false,
		reqFunc: func() (string, *string, error) {
			// Step 1: Send Solicit and receive the server's Advertise packet
			advertiseMsg, err := lm.client6.Solicit(lm.ctx)
			if err != nil {
				return "", nil, fmt.Errorf("failed to send DHCPv6 SOLICIT: %w", err)
			}
			if advertiseMsg == nil {
				return "", nil, fmt.Errorf("DHCPv6 ADVERTISE response cannot be nil")
			}

			// Step 2: Send Request using the received Advertise message to get the Reply
			packet6, err := lm.client6.Request(lm.ctx, advertiseMsg)
			if err != nil {
				return "", nil, fmt.Errorf("failed to get DHCPv6 lease via REQUEST: %w", err)
			}

			ip6, err := ipAddrFromLeaseV6(packet6)
			if err != nil {
				return "", nil, err
			}

			var gwPtr *string

			lm.mu.Lock()
			lm.lease6 = packet6
			lm.mu.Unlock()

			return ip6, gwPtr, nil
		},
	}

	var tasks []ipTask
	switch lm.family {
	case networkv1.IPFamilyIPv4:
		tasks = []ipTask{v4Task}
	case networkv1.IPFamilyIPv6:
		tasks = []ipTask{v6Task}
	case networkv1.IPFamilyDualIPv4First:
		tasks = []ipTask{v4Task, v6Task}
	default:
		tasks = []ipTask{v4Task}
	}

	// 1. Gather the IPs and Gateway from your tasks
	var multiFamilyIPs networkv1.MultiFamilyIP
	var newState networkv1.NodeNetworkState

	for _, task := range tasks {
		ipAddr, gw, err := task.reqFunc()
		if err != nil {
			return nil, err // defer will trigger lm.Stop()
		}

		if task.isV4 {
			ipv4 := networkv1.IPAddr(ipAddr)
			multiFamilyIPs.IPv4 = &ipv4
			newState.IPv4 = ipAddr
			if gw != nil {
				newState.GatewayIPv4 = *gw
			}
		} else {
			ipv6 := networkv1.IPAddr(ipAddr)
			multiFamilyIPs.IPv6 = &ipv6
			newState.IPv6 = ipAddr
		}
	}

	// 2. Apply both addresses atomically using the robust multi-family method
	if err := lm.link.SetMultiFamilyIPAddresses(&multiFamilyIPs, lm.vlanID); err != nil {
		return nil, fmt.Errorf("failed to set multi-family IP addresses: %w", err)
	}

	lm.mu.Lock()
	lm.netState = newState
	lm.running = true
	success = true // Mark as successfully started so defer doesn't rollback
	lm.mu.Unlock()

	if lm.client4 != nil {
		go lm.renewLoopV4()
	}
	if lm.client6 != nil {
		go lm.renewLoopV6()
	}

	return &newState, nil
}

func renewalDelayV4(lease *dhcpv4.DHCPv4) time.Duration {
	if t1 := lease.IPAddressRenewalTime(0); t1 > 0 {
		return t1
	}
	if lt := lease.IPAddressLeaseTime(0); lt > 0 {
		return lt / 2
	}
	return 30 * time.Second
}

// Note: DHCPv4 renew/request failure is not propagated to the controller yet;
// it is logged for visibility and will be handled/reported later.
func (lm *LeaseManager) renewLoopV4() {
	// Cache the interface name for clean, common usage across the loop
	iface := lm.iface
	var lease *nclient4.Lease
	failedToApplyIP := false

	// Handle a single renewal tick action
	handleTick := func() {
		newLease, err := lm.client4.Renew(lm.ctx, lease)
		if err != nil {
			logrus.Warnf("Failed to renew DHCPv4 lease for interface %s, attempting fallback request: %v", iface, err)
			newLease, err = lm.client4.Request(lm.ctx)
			if err != nil {
				logrus.Errorf("Failed to request new DHCPv4 lease for interface %s (will retry next cycle): %v", iface, err)
				return
			}
		}

		ipStr, err := ipAddrFromLeaseV4(newLease)
		if err != nil {
			logrus.Errorf("Failed to parse IPv4 address from DHCPv4 lease on interface %s: %v", iface, err)
			return
		}

		gwStr := ""
		if routers := newLease.ACK.Router(); len(routers) > 0 && routers[0] != nil && !routers[0].IsUnspecified() {
			gwStr = routers[0].String()
		}

		// Update internal state and check if IP or gateway parameters changed
		lm.mu.Lock()
		leaseChanged := lm.netState.IPv4 != ipStr || lm.netState.GatewayIPv4 != gwStr
		lm.lease4 = newLease // always updates it with latest lease
		lm.netState.IPv4 = ipStr
		lm.netState.GatewayIPv4 = gwStr
		lm.mu.Unlock()

		// If the IP and gateway are unchanged, skip low-level kernel netlink manipulations to avoid unnecessary churn.
		if !leaseChanged && !failedToApplyIP {
			logrus.Debugf("DHCPv4 lease for interface %s unchanged (IP: %s, Gateway: %s), skipping netlink re-application", iface, ipStr, gwStr)
			return
		}

		// Reuse existing SetMultiFamilyIPAddresses, but only set IPv4 family IP
		ipAddr := networkv1.IPAddr(ipStr)
		multiIP := &networkv1.MultiFamilyIP{
			IPv4: &ipAddr,
		}

		if err := lm.link.SetMultiFamilyIPAddresses(multiIP, lm.vlanID); err != nil {
			logrus.Errorf("Failed to apply DHCPv4 lease for interface %s (IP: %s, Gateway: %s): %v", iface, ipStr, gwStr, err)
			failedToApplyIP = true
		} else {
			logrus.Infof("Successfully applied DHCPv4 lease for interface %s (IP: %s, Gateway: %s)", iface, ipStr, gwStr)
			failedToApplyIP = false
		}

	}

	for {
		// Use RLock for reading lease4
		lm.mu.RLock()
		lease = lm.lease4
		lm.mu.RUnlock()

		// In the normal execution path, the lease will never be nil because the renewal
		// loop is only started after an initial lease is successfully acquired. This check
		// acts as a safeguard in case the lease manager's state is accidentally set to nil
		// directly from elsewhere.
		if lease == nil {
			logrus.Warnf("renewLoopV4 for interface %s encountered a nil lease, exiting renewal loop", iface)
			return
		}

		delay := renewalDelayV4(lease.ACK)
		timer := time.NewTimer(delay)

		select {
		case <-timer.C:
			handleTick()
			timer.Stop()
		case <-lm.ctx.Done():
			timer.Stop()
			return
		}
	}
}

func renewalDelayV6(packet dhcpv6.DHCPv6) time.Duration {
	if msg, ok := packet.(*dhcpv6.Message); ok {
		for _, ianaOpt := range msg.Options.IANA() {
			// ianaOpt.Options is of type IdentityOptions, which provides Addresses()
			for _, addrOpt := range ianaOpt.Options.Addresses() {
				if addrOpt.PreferredLifetime > 0 {
					return addrOpt.PreferredLifetime / 2
				}
				if addrOpt.ValidLifetime > 0 {
					return addrOpt.ValidLifetime / 4
				}
			}
		}
	}

	// Fallback default if lifetimes aren't present
	return 30 * time.Minute
}

// Note: DHCPv6 renew/request failure is not propagated to the controller yet;
// it is logged for visibility and will be handled/reported later.
func (lm *LeaseManager) renewLoopV6() {
	// Cache the interface name for clean, common usage across the loop
	iface := lm.iface
	var lease dhcpv6.DHCPv6
	failedToApplyIP := false

	// Handle a single renewal tick action
	handleTickV6 := func() {
		var newPacket dhcpv6.DHCPv6
		var err error

		// Attempt DHCPv6 RENEW if we have an existing lease stored
		if lease != nil {
			if replyMsg, ok := lease.(*dhcpv6.Message); ok {
				clientID := replyMsg.Options.ClientID()
				serverID := replyMsg.Options.ServerID()
				ianas := replyMsg.Options.IANA()

				if clientID != nil && serverID != nil && len(ianas) > 0 {
					modifiers := make([]dhcpv6.Modifier, 0, 2+len(ianas))
					modifiers = append(modifiers,
						dhcpv6.WithClientID(clientID),
						dhcpv6.WithServerID(serverID),
					)
					for _, iana := range ianas {
						ianaOpt := iana
						modifiers = append(modifiers, dhcpv6.WithOption(ianaOpt))
					}

					renewMsg, buildErr := dhcpv6.NewMessage(modifiers...)
					if buildErr == nil {
						renewMsg.MessageType = dhcpv6.MessageTypeRenew
						newPacket, err = lm.client6.SendAndRead(lm.ctx, nil, renewMsg, nil)
					} else {
						err = buildErr
					}
				} else {
					err = fmt.Errorf("missing client ID, server ID, or IANA in existing lease for renewal")
				}
			} else {
				err = fmt.Errorf("invalid lease type stored for renewal")
			}
		} else {
			err = fmt.Errorf("no existing lease available for renewal")
		}

		// Fallback to initial reacquisition (Solicit + Request) if RENEW failed or no lease existed
		if err != nil {
			logrus.Warnf("DHCPv6 RENEW failed for interface %s, falling back to initial reacquisition (Solicit/Request): %v", iface, err)

			advertiseMsg, solErr := lm.client6.Solicit(lm.ctx)
			if solErr != nil {
				logrus.Errorf("Failed to send DHCPv6 SOLICIT during reacquisition fallback on interface %s: %v", iface, solErr)
				return
			}
			if advertiseMsg == nil {
				logrus.Errorf("DHCPv6 ADVERTISE response cannot be nil during reacquisition fallback on interface %s", iface)
				return
			}

			newPacket, err = lm.client6.Request(lm.ctx, advertiseMsg)
			if err != nil {
				logrus.Errorf("Failed to get DHCPv6 lease via REQUEST during reacquisition fallback on interface %s: %v", iface, err)
				return
			}
		}

		ipStr, err := ipAddrFromLeaseV6(newPacket)
		if err != nil {
			logrus.Errorf("Failed to parse IPv6 address from DHCPv6 lease on interface %s: %v", iface, err)
			return
		}

		// Update internal state and check if IPv6 parameter changed
		lm.mu.Lock()
		leaseChanged := lm.netState.IPv6 != ipStr
		lm.lease6 = newPacket // always updates it with latest packet lease
		lm.netState.IPv6 = ipStr
		lm.mu.Unlock()

		// If the IPv6 address is unchanged, skip low-level kernel netlink manipulations to avoid unnecessary churn.
		if !leaseChanged && !failedToApplyIP {
			logrus.Debugf("DHCPv6 lease for interface %s unchanged (IP: %s), skipping netlink re-application", iface, ipStr)
			return
		}

		ipAddr := networkv1.IPAddr(ipStr)
		multiIP := &networkv1.MultiFamilyIP{
			IPv6: &ipAddr,
		}

		if err := lm.link.SetMultiFamilyIPAddresses(multiIP, lm.vlanID); err != nil {
			logrus.Errorf("Failed to apply DHCPv6 lease for interface %s (IP: %s): %v", iface, ipStr, err)
			failedToApplyIP = true
		} else {
			logrus.Infof("Successfully applied DHCPv6 lease for interface %s (IP: %s)", iface, ipStr)
			failedToApplyIP = false
		}
	}

	for {
		// Use RLock for reading lease6
		lm.mu.RLock()
		lease = lm.lease6
		lm.mu.RUnlock()

		delay := 30 * time.Minute
		if lease != nil {
			delay = renewalDelayV6(lease)
		}

		timer := time.NewTimer(delay)

		select {
		case <-timer.C:
			handleTickV6()
			timer.Stop()
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
	// If it's not running AND has no active context or leases, nothing to clean up.
	if !lm.running && lm.cancel == nil && lm.lease4 == nil && lm.lease6 == nil {
		lm.mu.Unlock()
		return
	}

	cancel := lm.cancel
	lease4 := lm.lease4

	lm.running = false
	lm.cancel = nil
	lm.lease4 = nil
	lm.lease6 = nil
	lm.netState = networkv1.NodeNetworkState{}
	lm.mu.Unlock()

	// 1. Cancel the long-running go thread to avoid conflicts with renewLoop
	if cancel != nil {
		cancel()
		// Give background goroutines a brief moment to unwind and release sockets
		time.Sleep(100 * time.Millisecond)
	}

	// 2. Best-effort IPv4 unicast release using the dedicated temporary client.
	// This must happen BEFORE closing the main client/socket.
	if lease4 != nil {
		if err := lm.ReleaseIPv4Lease(lease4); err != nil {
			logrus.Warnf("Failed to release DHCPv4 lease (proceeding anyway): %v", err)
		}
	}

	// 3. Clean up the main long-lived DHCP clients/sockets
	if lm.client4 != nil {
		_ = lm.client4.Close()
	}
	if lm.client6 != nil {
		// Note: nclient6.Client does not have a native .Release() method.
		_ = lm.client6.Close()
	}
}

func ipAddrFromLeaseV4(lease *nclient4.Lease) (string, error) {
	maskOpt := lease.ACK.Options.Get(dhcpv4.OptionSubnetMask)
	if maskOpt == nil {
		return "", fmt.Errorf("subnet mask not provided by DHCPv4 server")
	}

	if len(maskOpt) != 4 {
		return "", fmt.Errorf("invalid subnet mask length: %d", len(maskOpt))
	}

	ipMask := net.IPMask(maskOpt)
	ones, bits := ipMask.Size()
	if bits != 32 {
		return "", fmt.Errorf("invalid IPv4 subnet mask")
	}

	return fmt.Sprintf("%s/%d", lease.ACK.YourIPAddr.String(), ones), nil
}

// An IA_NA address does not carry an on-link prefix length, use `/128`
func ipAddrFromLeaseV6(packet dhcpv6.DHCPv6) (string, error) {
	msg, err := packet.GetInnerMessage()
	if err != nil {
		return "", err
	}

	ias := msg.Options.IANA()
	if len(ias) == 0 {
		return "", fmt.Errorf("no IA_NA options provided by DHCPv6 server")
	}

	for _, iana := range ias {
		addrs := iana.Options.Addresses()
		if len(addrs) > 0 && addrs[0].IPv6Addr != nil {
			return fmt.Sprintf("%s/128", addrs[0].IPv6Addr.String()), nil
		}
	}

	return "", fmt.Errorf("no IPv6 address found in DHCPv6 IA_NA options")
}

func (lm *LeaseManager) IsRunning() bool {
	lm.mu.RLock()
	defer lm.mu.RUnlock()
	return lm.running
}

func (lm *LeaseManager) GetNetworkState() networkv1.NodeNetworkState {
	lm.mu.RLock()
	defer lm.mu.RUnlock()
	return lm.netState
}

// ReleaseIPv4Lease sends a DHCPv4 RELEASE message to the server using a temporary unicast client.
// per RFC2131, section 4.4.4.
func (lm *LeaseManager) ReleaseIPv4Lease(lease *nclient4.Lease) error {
	if lease == nil {
		return nil
	}

	assignedIP := lease.ACK.YourIPAddr
	if assignedIP == nil || assignedIP.IsUnspecified() {
		return fmt.Errorf("invalid leased IP for release")
	}

	// Create a temporary client bound to the leased IP for unicast release
	client, err := nclient4.New(
		lm.iface,
		nclient4.WithUnicast(&net.UDPAddr{
			IP:   assignedIP,
			Port: nclient4.ClientPort,
		}),
	)
	if err != nil {
		return fmt.Errorf("failed to create unicast client for release: %w", err)
	}
	defer client.Close()

	// Send the release request
	if err := client.Release(lease); err != nil {
		return fmt.Errorf("failed to release IPv4 lease: %w", err)
	}

	logrus.Infof("successfully sent DHCPRELEASE for IP %s on interface %s", assignedIP.String(), lm.iface)
	return nil
}
