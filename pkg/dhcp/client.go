// implement DHCP client according to https://tools.ietf.org/html/rfc2131
package dhcp

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/insomniacslk/dhcp/dhcpv4/nclient4"
	"k8s.io/klog"
)

const defaultLeaseSeconds = 86400
const defaultRetryTimes = 10
const defaultRetryInterval = 3 * time.Second

// Client is a DHCP client, responsible for maintaining ipv4 lease for one specified interface
type Client struct {
	iface string

	broadcast *nclient4.Client

	lease   *nclient4.Lease
	stop    chan bool
	isStop  bool
	ackChan chan *dhcpv4.DHCPv4
}

func NewClient(iface string) *Client {
	return &Client{
		iface:   iface,
		stop:    make(chan bool),
		isStop:  true,
		ackChan: make(chan *dhcpv4.DHCPv4, 1),
	}
}

// Stop state-transition process and close dhcp client
func (c *Client) Stop() {
	c.stop <- true
}

// Start state-transition process of dhcp client
func (c *Client) Start() {
	var err error
	c.broadcast, err = nclient4.New(c.iface)
	if err != nil {
		klog.Errorf("create a broadcast client for iface %s failed, error: %s", c.iface, err.Error())
		return
	}
	defer c.broadcast.Close()

	c.isStop = false
	for i := 0; i < defaultRetryTimes; i++ {
		// DHCP State-transition: INIT --> BOUND
		lease, err := c.broadcast.Request(context.TODO(), dhcpv4.WithLeaseTime(defaultLeaseSeconds))
		if err != nil {
			klog.Errorf("Discover-Offer-Request-Ack handshake failed, error: %s", err.Error())
			time.Sleep(defaultRetryInterval)
			continue
		}
		c.lease = lease
		klog.Infof("discover-offer-request-ack, lease: %+v", lease)
		c.clearAckChan()
		c.ackChan <- c.lease.ACK

		// Set up two ticker to renew release regularly
		t1Timeout := c.lease.ACK.IPAddressLeaseTime(0) / 2
		t2Timeout := c.lease.ACK.IPAddressLeaseTime(0) / 8 * 7

		klog.Infof("t1: %s, t2: %s", t1Timeout, t2Timeout)
		t1, t2 := time.NewTicker(t1Timeout), time.NewTicker(t2Timeout)

		for {
			select {
			case <-t1.C:
				// renew
				lease, err := c.renew()
				if err == nil {
					c.lease = lease
					klog.Infof("renew, lease: %+v", lease)
					t2.Reset(t2Timeout)
				} else {
					klog.Errorf("renew failed, error: %s", err.Error())
				}
			case <-t2.C:
				// rebind
				lease, err := c.rebind()
				if err == nil {
					c.lease = lease
					klog.Infof("rebind, lease: %+v", lease)
					t1.Reset(t1Timeout)
				} else {
					klog.Errorf("rebind failed, error: %s", err.Error())
					t1.Stop()
					t2.Stop()
					break
				}
			case c.isStop = <-c.stop:
				if err := c.release(); err != nil {
					klog.Errorf("release lease failed, error: %s, lease: %+v", err.Error(), c.lease)
				}
				klog.Infof("release, lease: %+v", c.lease)
				t1.Stop()
				t2.Stop()
				return
			}
		}
	}
}

func (c *Client) release() error {
	unicast, err := nclient4.New(c.iface, nclient4.WithUnicast(&net.UDPAddr{Port: nclient4.ClientPort}),
		nclient4.WithServerAddr(&net.UDPAddr{IP: c.lease.ACK.ServerIPAddr, Port: nclient4.ServerPort}))
	if err != nil {
		return fmt.Errorf("create unicast client failed, error: %w, server ip: %v", err, c.lease.ACK.ServerIPAddr)
	}
	defer unicast.Close()

	// TODO modify lease
	return unicast.Release(c.lease)
}

// TODO Client messages is shown as follow. We should modify the DHCP package from lease before sending message.
//   ---------------------------------------------------------------------
//   |              |INIT-REBOOT  |SELECTING    |RENEWING     |REBINDING |
//   ---------------------------------------------------------------------
//   |broad/unicast |broadcast    |broadcast    |unicast      |broadcast |
//   |server-ip     |MUST NOT     |MUST         |MUST NOT     |MUST NOT  |
//   |requested-ip  |MUST         |MUST         |MUST NOT     |MUST NOT  |
//   |ciaddr        |zero         |zero         |IP address   |IP address|
//   ---------------------------------------------------------------------
func (c *Client) renew() (*nclient4.Lease, error) {
	unicast, err := nclient4.New(c.iface, nclient4.WithUnicast(&net.UDPAddr{Port: nclient4.ClientPort}),
		nclient4.WithServerAddr(&net.UDPAddr{IP: c.lease.ACK.ServerIPAddr, Port: nclient4.ServerPort}))
	if err != nil {
		return nil, fmt.Errorf("create unicast client failed, error: %w, server ip: %v", err, c.lease.ACK.ServerIPAddr)
	}
	defer unicast.Close()

	// TODO modify offer
	newLease, err := unicast.RequestFromOffer(context.TODO(), c.lease.ACK, dhcpv4.WithLeaseTime(defaultLeaseSeconds))
	if err != nil {
		return nil, fmt.Errorf("request to leasing server failed, error: %w, offer: %+v", err, c.lease.ACK)
	}

	return newLease, nil
}

func (c *Client) rebind() (*nclient4.Lease, error) {
	// TODO modify offer
	newLease, err := c.broadcast.RequestFromOffer(context.TODO(), c.lease.ACK, dhcpv4.WithLeaseTime(defaultLeaseSeconds))
	if err != nil {
		return nil, fmt.Errorf("broadcast request failed, error: %w, offer: %+v", err, c.lease.ACK)
	}

	return newLease, nil
}

func (c *Client) GetIPv4Addr() (net.IP, net.IPMask, net.IP, error) {
	select {
	case ack := <-c.ackChan:
		return ack.YourIPAddr, ack.SubnetMask(), ack.GatewayIPAddr, nil
	case <-time.After(defaultRetryTimes * defaultRetryInterval):
		return nil, nil, nil, fmt.Errorf("timeout")
	}
}

func (c *Client) clearAckChan() {
	for len(c.ackChan) > 0 {
		<-c.ackChan
	}
}

func (c *Client) IsRunning() bool {
	return !c.isStop
}
