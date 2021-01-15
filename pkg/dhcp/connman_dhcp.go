package dhcp

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"k8s.io/klog"
)

// Networking is powered by connman in k3os.
// Thus, we use connmanctl to configure dhcp for physical nic

const (
	// keep --ipv4 to the end to avoid empty gateway configuration
	configDHCPCmd        = "connmanctl config %s --ipv4 %s"
	configNameserversCmd = "connmanctl config %s --nameservers 8.8.8.8"
	getIPv4ConfigCmd     = "connmanctl services %s"
	ipv4ConfigKeyword    = "IPv4.Configuration"
	ipv4MethodKeyword    = "Method"
	ipv4Keyword          = "IPv4 = "
	addrKeyword          = "Address"
	maskKeyword          = "Netmask"
	gatewayKeyword       = "Gateway"

	// IPv4 mode
	Off    = "off"
	DHCP   = "dhcp"
	Static = "manual"

	getServicesRetryInterval = 200 * time.Microsecond
	waitEffectiveTime        = 2 * time.Second
)

type ConnmanService struct {
	iface   string
	service string
}

func NewConnmanService(iface, hwaddr string) *ConnmanService {
	service := "ethernet_" + strings.Replace(hwaddr, ":", "", -1) + "_cable"

	return &ConnmanService{iface, service}
}

// ConfigIPv4 with connmanctl
func (c *ConnmanService) configIPv4(mode string, args ...string) error {
	if mode == Static {
		for _, arg := range args {
			mode += " "
			mode += arg
		}
	}

	var err error
	for i := 0; i < defaultRetryTimes; i++ {
		klog.Infof("check services, retry times: %d", i+1)
		_, err = c.getServicesDetails()
		if err != nil {
			time.Sleep(getServicesRetryInterval)
			continue
		}
		break
	}
	if err != nil {
		return fmt.Errorf("%s is not existed", c.service)
	}

	statement := fmt.Sprintf(configDHCPCmd, c.service, mode)
	klog.Infof("statement: %s", statement)
	out, err := exec.Command("sh", "-c", statement).CombinedOutput()
	if err != nil {
		return fmt.Errorf("execute connmanctl config failed, error: %w, cmd: %s", err, statement)
	}
	if len(out) != 0 {
		return fmt.Errorf("execute connmanctl config failed, stderr: [%s], cmd: %s", out, statement)
	}

	return nil
}

func (c *ConnmanService) configNameservers() error {
	statement := fmt.Sprintf(configNameserversCmd, c.service)
	out, err := exec.Command("sh", "-c", statement).CombinedOutput()
	if err != nil {
		return fmt.Errorf("execute connmanctl config failed, error: %w, cmd: %s", err, statement)
	}
	if len(out) != 0 {
		return fmt.Errorf("execute connmanctl config failed, stderr: [%s], cmd: %s", out, statement)
	}

	return nil
}

func (c *ConnmanService) DHCP2Static() error {
	out, err := c.getServicesDetails()
	if err != nil {
		return err
	}

	addr, mask, gw := getIPv4Addr(out)

	if err := c.configIPv4(Static, addr, mask, gw); err != nil {
		return err
	}

	if err := c.configNameservers(); err != nil {
		return err
	}

	// wait for becoming effective
	time.Sleep(waitEffectiveTime)

	return nil
}

func (c *ConnmanService) ToDHCP() error {
	if err := c.configIPv4(DHCP); err != nil {
		return err
	}

	if err := c.configNameservers(); err != nil {
		return err
	}

	// wait for becoming effective
	time.Sleep(waitEffectiveTime)

	return nil
}

// GetIPv4Config with connmanctl
func (c *ConnmanService) GetIPv4Mode() (string, error) {
	out, err := c.getServicesDetails()
	if err != nil {
		return "", err
	}

	return getIPv4Mode(out), nil
}

func (c *ConnmanService) getServicesDetails() (string, error) {
	statement := fmt.Sprintf(getIPv4ConfigCmd, c.service)
	cmd := exec.Command("sh", "-c", statement)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("run command %s failed, error: %w", statement, err)
	}

	if len(stderr.Bytes()) != 0 {
		return "", fmt.Errorf("run command %s failed, error: %s", statement, stderr.Bytes())
	}

	return stdout.String(), nil
}

func getIPv4Mode(str string) string {
	rows := strings.Split(str, "\n")

	for _, row := range rows {
		if strings.Contains(row, ipv4ConfigKeyword) {
			fields := strings.FieldsFunc(row, func(c rune) bool {
				return c == '[' || c == ']' || c == '=' || c == ' ' || c == ','
			})

			for i, field := range fields {
				if field == ipv4MethodKeyword && len(fields) > i+1 {
					return fields[i+1]
				}
			}
			break
		}
	}

	return ""
}

func getIPv4Addr(str string) (string, string, string) {
	var addr, mask, gateway string
	rows := strings.Split(str, "\n")

	for _, row := range rows {
		if strings.Contains(row, ipv4Keyword) {
			fields := strings.FieldsFunc(row, func(c rune) bool {
				return c == '[' || c == ']' || c == '=' || c == ' ' || c == ','
			})

			for i, field := range fields {
				if field == addrKeyword && len(fields) > i+1 {
					addr = fields[i+1]
				}
				if field == maskKeyword && len(fields) > i+1 {
					mask = fields[i+1]
				}
				if field == gatewayKeyword && len(fields) > i+1 {
					gateway = fields[i+1]
				}
			}
		}
	}

	return addr, mask, gateway
}
