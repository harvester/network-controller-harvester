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
	"github.com/insomniacslk/dhcp/iana"

	"github.com/harvester/harvester-network-controller/pkg/utils"
)

const (
	// Brief pause to allow the kernel to assign the IPv6 link-local address on the newly created interface
	ipv6SleepMillisecond   = 200
	cancelSleepMillisecond = 100
)

type LeaseManager struct {
	iface  string
	link   *iface.Link
	vlanID uint16
	family networkv1.IPFamily

	client4 *nclient4.Client
	client6 *nclient6.Client
	duid    *dhcpv6.DUIDLLT // only initialized and exists for IPv6 or Dual-Stack

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

// NewLeaseManager atomically handles the entire lifecycle of lease acquisition.
// It performs instance creation, socket/client initialization, DHCP lease requests,
// multi-family IP address application to the link, and background renewal loop spawning.
// Follows an all-or-nothing transactional pattern: if any step fails, StopLease() is
// automatically invoked via defer to clean up all allocated sockets and resources.
func NewLeaseManager(iface string, link *iface.Link, hnc *networkv1.HostNetworkConfig) (*LeaseManager, *networkv1.NodeNetworkState, error) {
	family := hnc.Spec.IPFamily
	if family == "" {
		family = networkv1.IPFamilyIPv4 // Default fallback
	}

	ctx, cancel := context.WithCancel(context.Background())
	lm := &LeaseManager{
		iface:  iface,
		link:   link,
		vlanID: hnc.Spec.VlanID,
		family: family,
		ctx:    ctx,
		cancel: cancel,
	}

	// Ensure all-or-nothing cleanup of clients/sockets if anything fails midway.
	// Parent function is solely responsible for cleanup during initialization.
	var success bool
	defer func() {
		if !success {
			lm.StopLease()
		}
	}()

	// 1. Initialize Network Clients based on IP family
	var err error
	switch family {
	case networkv1.IPFamilyIPv4:
		lm.client4, err = nclient4.New(iface)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create DHCPv4 client: %w", err)
		}
	case networkv1.IPFamilyIPv6:
		time.Sleep(ipv6SleepMillisecond * time.Millisecond)
		if err := lm.initDUID(hnc); err != nil {
			return nil, nil, err
		}
		lm.client6, err = nclient6.New(iface)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create DHCPv6 client: %w", err)
		}
	case networkv1.IPFamilyDualIPv4First:
		if err := lm.initDUID(hnc); err != nil {
			return nil, nil, err
		}
		lm.client4, err = nclient4.New(iface)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create DHCPv4 client: %w", err)
		}
		time.Sleep(ipv6SleepMillisecond * time.Millisecond)
		lm.client6, err = nclient6.New(iface)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create DHCPv6 client: %w", err)
		}
	default:
		return nil, nil, fmt.Errorf("unsupported network family %v", family)
	}

	// 2. Execute internal start logic: request leases, apply to link, and start background loops
	state, err := lm.start()
	if err != nil {
		return nil, nil, err
	}

	success = true
	return lm, state, nil
}

// start executes active lease requests, binds addresses, and spawns renewal loops.
// Since lm is locally owned during initialization, no locks are needed until background routines spawn.
func (lm *LeaseManager) start() (*networkv1.NodeNetworkState, error) {
	var multiIP iface.MultiFamilyIPWithLease
	var newState networkv1.NodeNetworkState

	type ipTask struct {
		reqFunc func() error
	}

	v4Task := ipTask{
		reqFunc: func() error {
			lease4, err := lm.client4.Request(lm.ctx)
			if err != nil {
				return fmt.Errorf("failed to get IPv4 lease: %w", err)
			}
			ip4, err := ipAddrFromLeaseV4(lease4)
			if err != nil {
				// not booked yet, rollback won't touch it, try to release explicitly
				_ = lm.client4.Release(lease4)
				return err
			}

			ipv4 := networkv1.IPAddr(ip4)
			multiIP.IPv4 = &ipv4
			newState.IPv4 = ip4

			if routers := lease4.ACK.Router(); len(routers) > 0 && routers[0] != nil && !routers[0].IsUnspecified() {
				newState.GatewayIPv4 = routers[0].String()
			}

			lm.lease4 = lease4
			return nil
		},
	}

	v6Task := ipTask{
		reqFunc: func() error {
			packet6, err := lm.initializeLeaseV6("lease start")
			if err != nil {
				return err
			}
			ip6, prefLft, validLft, err := ipAddrFromLeaseV6(packet6)
			if err != nil {
				return err
			}

			ipv6 := networkv1.IPAddr(ip6)
			multiIP.IPv6 = &ipv6
			multiIP.V6PreferredLifetime = prefLft
			multiIP.V6ValidLifetime = validLft
			newState.IPv6 = ip6

			// Directly assign without locking
			lm.lease6 = packet6
			return nil
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

	for _, task := range tasks {
		if err := task.reqFunc(); err != nil {
			return nil, err
		}
	}

	if err := lm.link.SetMultiFamilyIPAddresses(&multiIP, lm.vlanID); err != nil {
		return nil, fmt.Errorf("failed to set leased multi-family IP addresses: %w", err)
	}

	lm.netState = newState
	lm.running = true

	// Now that background loops are about to be spawned, they need to use lock to operate lm's data
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
	ifaceStr := lm.iface
	var lease *nclient4.Lease
	failedToApplyIP := false

	// Handle a single renewal tick action
	handleTick := func() {
		newLease, err := lm.client4.Renew(lm.ctx, lease)
		if err != nil {
			logrus.Warnf("Failed to renew DHCPv4 lease for interface %s, attempting fallback request: %v", ifaceStr, err)
			newLease, err = lm.client4.Request(lm.ctx)
			if err != nil {
				logrus.Errorf("Failed to request new DHCPv4 lease for interface %s (will retry next cycle): %v", ifaceStr, err)
				return
			}
		}

		ipStr, err := ipAddrFromLeaseV4(newLease)
		if err != nil {
			logrus.Errorf("Failed to parse IPv4 address from DHCPv4 lease on interface %s: %v", ifaceStr, err)
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
			logrus.Infof("DHCPv4 lease for interface %s unchanged (IP: %s, Gateway: %s), skipping netlink re-application", ifaceStr, ipStr, gwStr)
			return
		}

		// Reuse existing SetMultiFamilyIPAddresses, but only set IPv4 family IP
		ipAddr := networkv1.IPAddr(ipStr)
		if err := lm.link.SetMultiFamilyIPAddresses(iface.NewMultiFamilyIPWithLeaseFromIPv4(&ipAddr), lm.vlanID); err != nil {
			logrus.Errorf("Failed to apply DHCPv4 lease for interface %s (IP: %s, Gateway: %s): %v", ifaceStr, ipStr, gwStr, err)
			failedToApplyIP = true
		} else {
			logrus.Infof("Successfully applied DHCPv4 lease for interface %s (IP: %s, Gateway: %s)", ifaceStr, ipStr, gwStr)
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
			logrus.Warnf("renewLoopV4 for interface %s encountered a nil lease, exiting renewal loop", ifaceStr)
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
	ifaceStr := lm.iface
	var lease dhcpv6.DHCPv6

	// Handle a single renewal tick action
	handleTickV6 := func() {
		var newPacket dhcpv6.DHCPv6
		var err error

		// Attempt DHCPv6 RENEW if there is an existing lease stored
		if lease == nil {
			err = fmt.Errorf("no existing lease available for renewal")
		} else {
			var renewMsg *dhcpv6.Message
			renewMsg, err = lm.buildRenewPacketV6(lease)
			if err == nil {
				// Explicitly target the DHCPv6 All Servers multicast address scoped to our interface
				dstAddr := &net.UDPAddr{
					IP:   dhcpv6.AllDHCPRelayAgentsAndServers, // ff02::1:2
					Port: dhcpv6.DefaultServerPort,            // 547
					Zone: ifaceStr,                            // e.g. "mgmt-br.100"
				}
				newPacket, err = lm.client6.SendAndRead(lm.ctx, dstAddr, renewMsg, nil)
			}
		}

		// Fallback to initial reacquisition (Solicit + Request) if RENEW failed or no lease existed
		if err != nil {
			logrus.Warnf("DHCPv6 RENEW failed for interface %s, falling back to initial reacquisition (Solicit/Request): %v", ifaceStr, err)
			newPacket, err = lm.initializeLeaseV6("lease renew")
			if err != nil {
				logrus.Errorf("Failed to get DHCPv6 lease via REQUEST during reacquisition fallback on interface %s: %v", ifaceStr, err)
				return
			}
		}

		ipStr, prefLft, validLft, err := ipAddrFromLeaseV6(newPacket)
		if err != nil {
			logrus.Errorf("Failed to parse IPv6 address from DHCPv6 lease on interface %s: %v", ifaceStr, err)
			return
		}

		// Update internal state and check if IPv6 parameter changed
		lm.mu.Lock()
		lm.lease6 = newPacket // always updates it with latest packet lease
		lm.netState.IPv6 = ipStr
		lm.mu.Unlock()

		ipAddr := networkv1.IPAddr(ipStr)
		if err := lm.link.SetMultiFamilyIPAddresses(iface.NewMultiFamilyIPWithLeaseFromIPv6(&ipAddr, prefLft, validLft), lm.vlanID); err != nil {
			logrus.Errorf("Failed to apply DHCPv6 lease for interface %s (IP: %s): %v", ifaceStr, ipStr, err)
		} else {
			logrus.Infof("Successfully applied DHCPv6 lease for interface %s (IP: %s)", ifaceStr, ipStr)
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

func (lm *LeaseManager) StopLease() {
	if lm == nil {
		return
	}

	lm.mu.Lock()
	if !lm.running {
		lm.mu.Unlock()
		return
	}

	lm.running = false
	cancel := lm.cancel
	lease4 := lm.lease4
	client4 := lm.client4
	client6 := lm.client6

	// Clear internal references under lock
	lm.cancel = nil
	lm.lease4 = nil
	lm.lease6 = nil
	lm.netState = networkv1.NodeNetworkState{}
	lm.mu.Unlock()

	// 1. Cancel the long-running go thread to avoid conflicts with renewLoop
	if cancel != nil {
		cancel()
		// Give background goroutines a brief moment to unwind and release sockets
		time.Sleep(cancelSleepMillisecond * time.Millisecond)
	}

	// 2. Best-effort IPv4 unicast release using the dedicated temporary client.
	// This must happen BEFORE closing the main client/socket.
	if lease4 != nil {
		if err := lm.ReleaseIPv4Lease(lease4); err != nil {
			logrus.Warnf("Failed to release DHCPv4 lease (proceeding anyway): %v", err)
		}
	}

	// 3. Close sockets if present
	if client4 != nil {
		_ = client4.Close()
	}
	if client6 != nil {
		_ = client6.Close()
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

// ipAddrFromLeaseV6 extracts the IPv6 address and its lease lifetimes from IA_NA options.
// the IA_NA address does not carry an on-link prefix length, hence use `/128`
func ipAddrFromLeaseV6(packet dhcpv6.DHCPv6) (string, time.Duration, time.Duration, error) {
	msg, err := packet.GetInnerMessage()
	if err != nil {
		return "", 0, 0, err
	}

	ias := msg.Options.IANA()
	if len(ias) == 0 {
		return "", 0, 0, fmt.Errorf("no IA_NA options provided by DHCPv6 server")
	}

	for _, iana := range ias {
		addrs := iana.Options.Addresses()
		if len(addrs) > 0 && addrs[0].IPv6Addr != nil {
			addr := addrs[0]
			ipStr := fmt.Sprintf("%s/128", addr.IPv6Addr.String())
			return ipStr, addr.PreferredLifetime, addr.ValidLifetime, nil
		}
	}

	return "", 0, 0, fmt.Errorf("no IPv6 address found in DHCPv6 IA_NA options")
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

func (lm *LeaseManager) initDUID(hnc *networkv1.HostNetworkConfig) error {
	if lm.link == nil || lm.link.Attrs() == nil {
		return fmt.Errorf("link or link attributes are not initialized for DUID generation")
	}
	hwAddr := lm.link.Attrs().HardwareAddr
	if len(hwAddr) == 0 {
		return fmt.Errorf("interface %s has no hardware address for DUID generation", lm.iface)
	}

	// RFC 8415 DUID-LLT epoch: Jan 1, 2000 00:00:00 UTC (Unix timestamp 946684800)
	creationTime := hnc.CreationTimestamp
	var duidTime uint32
	if !creationTime.IsZero() {
		unixSecs := creationTime.Unix()
		if unixSecs > utils.DUIDLLTEPOCHSeconds {
			delta := unixSecs - utils.DUIDLLTEPOCHSeconds
			// Ensure delta fits within uint32 max value to satisfy gosec G115
			if delta > 0 && delta <= 0xFFFFFFFF {
				duidTime = uint32(delta)
			}
		}
	}

	lm.duid = &dhcpv6.DUIDLLT{
		HWType:        iana.HWTypeEthernet,
		Time:          duidTime,
		LinkLayerAddr: hwAddr,
	}
	return nil
}

func (lm *LeaseManager) initializeLeaseV6(scenario string) (dhcpv6.DHCPv6, error) {
	// Build modifiers ensuring our stable persistent DUID is always sent
	var clientModifiers []dhcpv6.Modifier
	if lm.duid != nil {
		clientModifiers = append(clientModifiers, dhcpv6.WithClientID(lm.duid))
	}

	// Step 1: Send Solicit and receive the server's Advertise packet
	advertiseMsg, err := lm.client6.Solicit(lm.ctx, clientModifiers...)
	if err != nil {
		return nil, fmt.Errorf("%s failed to send DHCPv6 SOLICIT: %w", scenario, err)
	}
	if advertiseMsg == nil {
		return nil, fmt.Errorf("%s got an empty DHCPv6 ADVERTISE", scenario)
	}

	// Step 2: Send Request using the received Advertise message to get the Reply
	packet6, err := lm.client6.Request(lm.ctx, advertiseMsg, clientModifiers...)
	if err != nil {
		return nil, fmt.Errorf("%s failed to get DHCPv6 lease via REQUEST: %w", scenario, err)
	}

	return packet6, nil
}

func (lm *LeaseManager) buildRenewPacketV6(lease dhcpv6.DHCPv6) (*dhcpv6.Message, error) {
	if lease == nil {
		return nil, fmt.Errorf("no existing lease available for renewal")
	}

	replyMsg, ok := lease.(*dhcpv6.Message)
	if !ok {
		return nil, fmt.Errorf("invalid lease type stored for renewal")
	}

	serverID := replyMsg.Options.ServerID()
	ianas := replyMsg.Options.IANA()

	if serverID == nil || len(ianas) == 0 {
		return nil, fmt.Errorf("missing server ID or IANA in existing lease for renewal")
	}

	modifiers := make([]dhcpv6.Modifier, 0, 3+len(ianas))
	modifiers = append(modifiers,
		dhcpv6.WithClientID(lm.duid), // Reuse persistent stable DUID
		dhcpv6.WithServerID(serverID),
		dhcpv6.WithOption(dhcpv6.OptElapsedTime(0)), // RFC 3315/8415 mandated option OPTION_ELAPSED_TIME
	)

	for _, iana := range ianas {
		modifiers = append(modifiers, dhcpv6.WithOption(iana))
	}

	renewMsg, err := dhcpv6.NewMessage(modifiers...)
	if err != nil {
		return nil, fmt.Errorf("failed to build DHCPv6 RENEW message: %w", err)
	}

	renewMsg.MessageType = dhcpv6.MessageTypeRenew
	return renewMsg, nil
}
