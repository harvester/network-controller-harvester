package dhcp

import "testing"

var str1 = `
/net/connman/service/ethernet_000c295b4482_cable
  Type = ethernet
  Security = [  ]
  State = configuration
  Favorite = True
  Immutable = False
  AutoConnect = True
  Name = Wired
  Ethernet = [ Method=auto, Interface=eth0, Address=00:0C:29:5B:44:82, MTU=1500 ]
  IPv4 = [  ]
  IPv4.Configuration = [ Method=off ]
  IPv6 = [  ]
  IPv6.Configuration = [ Method=auto, Privacy=disabled ]
  Nameservers = [  ]
  Nameservers.Configuration = [  ]
  Timeservers = [  ]
  Timeservers.Configuration = [  ]
  Domains = [  ]
  Domains.Configuration = [  ]
  Proxy = [  ]
  Proxy.Configuration = [  ]
  mDNS = False
  mDNS.Configuration = False
  Provider = [  ]
`
var str2 = `
/net/connman/service/ethernet_000c295b4482_cable
  Type = ethernet
  Security = [  ]
  State = ready
  Favorite = True
  Immutable = False
  AutoConnect = True
  Name = Wired
  Ethernet = [ Method=auto, Interface=eth0, Address=00:0C:29:5B:44:82, MTU=1500 ]
  IPv4 = [ Method=manual, Address=172.16.0.238, Netmask=255.255.0.0, Gateway=172.16.0.1 ]
  IPv4.Configuration = [ Method=manual, Address=172.16.0.238, Netmask=255.255.0.0, Gateway=172.16.0.1 ]
  IPv6 = [  ]
  IPv6.Configuration = [ Method=auto, Privacy=disabled ]
  Nameservers = [ 8.8.8.8 ]
  Nameservers.Configuration = [  ]
  Timeservers = [ ntp.ubuntu.com ]
  Timeservers.Configuration = [  ]
  Domains = [  ]
  Domains.Configuration = [  ]
  Proxy = [ Method=direct ]
  Proxy.Configuration = [  ]
  mDNS = False
  mDNS.Configuration = False
  Provider = [  ]
`

func Test_getIPv4Mode(t *testing.T) {
	type args struct {
		str string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{"case 1", args{str: str1}, "off"},
		{"case 2", args{str: str2}, "manual"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getIPv4Mode(tt.args.str); got != tt.want {
				t.Errorf("getIPv4Mode() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_getIPv4Addr(t *testing.T) {
	type args struct {
		str string
	}
	tests := []struct {
		name  string
		args  args
		want  string
		want1 string
		want2 string
	}{
		{"case 1", args{str: str1}, "", "", ""},
		{"case 2", args{str: str2}, "172.16.0.238", "255.255.0.0", "172.16.0.1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1, got2 := getIPv4Addr(tt.args.str)
			if got != tt.want {
				t.Errorf("getIPv4Addr() got = %v, want %v", got, tt.want)
			}
			if got1 != tt.want1 {
				t.Errorf("getIPv4Addr() got1 = %v, want %v", got1, tt.want1)
			}
			if got2 != tt.want2 {
				t.Errorf("getIPv4Addr() got2 = %v, want %v", got2, tt.want2)
			}
		})
	}
}
