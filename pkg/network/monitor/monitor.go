package monitor

import (
	"context"
	"sync"
	"syscall"

	"github.com/vishvananda/netlink"
	"k8s.io/klog"
)

// netlink event type
// link:  RTM_NEWLINK  RTM_DELLINK
// addr:  RTM_NEWADDR  RTM_DELADDR
// route: RTM_NEWROUTE RTM_DELROUTE

type Monitor struct {
	handlers map[int]Handler
	mutex    sync.RWMutex

	done chan struct{}

	// every monitor can start/stop once
	startOnce sync.Once
}

func NewMonitor() *Monitor {
	return &Monitor{
		handlers:  make(map[int]Handler),
		mutex:     sync.RWMutex{},
		done:      make(chan struct{}),
		startOnce: sync.Once{},
	}
}

type Handler struct {
	NewLink  func(link netlink.LinkUpdate)
	DelLink  func(link netlink.LinkUpdate)
	NewAddr  func(addr netlink.AddrUpdate)
	DelAddr  func(addr netlink.AddrUpdate)
	NewRoute func(route netlink.RouteUpdate)
	DelRoute func(route netlink.RouteUpdate)
}

func (m *Monitor) Start(ctx context.Context) {
	m.startOnce.Do(func() {
		m.start(ctx)
	})
}

func (m *Monitor) start(ctx context.Context) {
	klog.Info("Start Monitor")
	linkCh := make(chan netlink.LinkUpdate)
	routeCh := make(chan netlink.RouteUpdate)
	addrCh := make(chan netlink.AddrUpdate)

	if err := netlink.LinkSubscribe(linkCh, ctx.Done()); err != nil {
		klog.Errorf("subscribe link failed, error: %s", err.Error())
		return
	}
	if err := netlink.RouteSubscribe(routeCh, ctx.Done()); err != nil {
		klog.Errorf("subscribe route failed, error: %s", err.Error())
		return
	}
	if err := netlink.AddrSubscribe(addrCh, ctx.Done()); err != nil {
		klog.Errorf("subscribe addr failed, error: %s", err.Error())
		return
	}

	go func() {
		for {
			select {
			case l := <-linkCh:
				m.handleLink(l)
			case a := <-addrCh:
				m.handleAddr(a)
			case r := <-routeCh:
				m.handleRoute(r)
			}
		}
	}()

	<-ctx.Done()
}

func (m *Monitor) handleLink(update netlink.LinkUpdate) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	handler, ok := m.handlers[int(update.Index)]
	if !ok {
		return
	}

	switch update.Header.Type {
	case syscall.RTM_NEWLINK:
		if handler.NewLink != nil {
			handler.NewLink(update)
		}
	case syscall.RTM_DELLINK:
		if handler.DelLink != nil {
			handler.DelLink(update)
		}
	default:

	}
}

func (m *Monitor) handleAddr(update netlink.AddrUpdate) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	handler, ok := m.handlers[int(update.LinkIndex)]
	if !ok {
		return
	}

	// ignore IPv6
	if ip := update.LinkAddress.IP.To4(); ip == nil {
		return
	}

	if update.NewAddr {
		if handler.NewAddr != nil {
			handler.NewAddr(update)
		}
	} else {
		if handler.DelAddr != nil {
			handler.DelAddr(update)
		}
	}
}

func (m *Monitor) handleRoute(update netlink.RouteUpdate) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	handler, ok := m.handlers[int(update.LinkIndex)]
	if !ok {
		return
	}
	switch update.Type {
	case syscall.RTM_NEWROUTE:
		if handler.NewRoute != nil {
			handler.NewRoute(update)
		}
	case syscall.RTM_DELROUTE:
		if handler.DelRoute != nil {
			handler.DelRoute(update)
		}
	default:

	}
}

func (m *Monitor) AddLink(index int, handler Handler) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.handlers[index] = handler
}

func (m *Monitor) DelLink(index int) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	delete(m.handlers, index)
}

func (m *Monitor) EmptyLink() {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.handlers = make(map[int]Handler)
}
