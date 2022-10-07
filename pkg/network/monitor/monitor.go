package monitor

import (
	"context"
	"regexp"
	"sync"
	"syscall"

	"github.com/vishvananda/netlink"
	"k8s.io/klog/v2"

	"github.com/harvester/harvester-network-controller/pkg/network/iface"
)

type Monitor struct {
	rule  map[string]*Pattern
	mutex sync.RWMutex

	handler *Handler

	done chan struct{}
	// every monitor can start/stop once
	startOnce sync.Once
}

type Pattern struct {
	Type string
	Name string
}

func NewPattern(typeRegexp, nameRegexp string) *Pattern {
	return &Pattern{
		Type: typeRegexp,
		Name: nameRegexp,
	}
}

func NewMonitor(h *Handler) *Monitor {
	return &Monitor{
		rule:      make(map[string]*Pattern),
		mutex:     sync.RWMutex{},
		handler:   h,
		done:      make(chan struct{}),
		startOnce: sync.Once{},
	}
}

type Handler struct {
	NewLink func(key string, update *netlink.LinkUpdate) error
	DelLink func(key string, update *netlink.LinkUpdate) error
}

func (m *Monitor) Start(ctx context.Context) {
	m.startOnce.Do(func() {
		m.start(ctx)
	})
}

func (m *Monitor) start(ctx context.Context) {
	klog.Info("Start Monitor")
	linkCh := make(chan netlink.LinkUpdate)

	if err := netlink.LinkSubscribe(linkCh, ctx.Done()); err != nil {
		klog.Errorf("subscribe link failed, error: %s", err.Error())
		return
	}

	for {
		select {
		case l := <-linkCh:
			if err := m.handleLink(&l); err != nil {
				klog.Errorf("monitor handles link %s failed, error: %s", l.Link.Attrs().Name, err.Error())
			}
		case <-ctx.Done():
			return
		}
	}
}

func (m *Monitor) handleLink(update *netlink.LinkUpdate) error {
	klog.V(5).Infof("netlink event: %+v", update)

	ok, key, err := m.match(update.Link)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}

	// link update message type: RTM_NEWLINK  RTM_DELLINK
	switch update.Header.Type {
	case syscall.RTM_NEWLINK:
		if m.handler.NewLink != nil {
			if err := m.handler.NewLink(key, update); err != nil {
				return err
			}
		}
	case syscall.RTM_DELLINK:
		if m.handler.DelLink != nil {
			if err := m.handler.DelLink(key, update); err != nil {
				return err
			}
		}
	default:

	}

	return nil
}

func (m *Monitor) match(l netlink.Link) (bool, string, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	for name, pattern := range m.rule {
		isMatch, err := pattern.match(l)
		if err != nil {
			return false, "", err
		}
		if isMatch {
			return true, name, nil
		}
	}

	return false, "", nil
}

func (m *Monitor) AddPattern(key string, pattern *Pattern) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.rule[key] = pattern
}

func (m *Monitor) ScanLinks(pattern *Pattern) ([]netlink.Link, error) {
	links, err := netlink.LinkList()
	if err != nil {
		return nil, err
	}

	matchingLinks := make([]netlink.Link, 0, len(links))
	for _, link := range links {
		if ok, err := pattern.match(link); err != nil {
			return nil, err
		} else if ok {
			matchingLinks = append(matchingLinks, link)
		}
	}

	return matchingLinks, nil
}

func (m *Monitor) DeletePattern(key string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	delete(m.rule, key)
}

func (m *Monitor) GetPattern(key string) *Pattern {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.rule[key]
}

func (p *Pattern) match(l netlink.Link) (bool, error) {
	var isMatchName, isMatchType bool
	var err error

	// always ignore loopback device
	if l.Attrs().EncapType == iface.TypeLoopback {
		return false, nil
	}

	if p.Name == "" {
		isMatchName = true
	} else {
		if isMatchName, err = regexp.MatchString(p.Name, l.Attrs().Name); err != nil {
			return false, err
		}
	}

	if p.Type == "" {
		isMatchType = true
	} else {
		if isMatchType, err = regexp.MatchString(p.Type, l.Type()); err != nil {
			return false, err
		}
	}

	return isMatchName && isMatchType, nil
}
