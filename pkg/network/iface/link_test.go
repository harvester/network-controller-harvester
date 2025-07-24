package iface

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vishvananda/netlink"
)

func TestLinkRelatedFunctions(t *testing.T) {
	tests := []struct {
		name      string
		returnErr bool
		errKey    string
		linkname  string
		vid       int
	}{
		{
			name:      "vid 0 on mgmt",
			returnErr: false,
			errKey:    "",
			linkname:  "mgmt-br",
			vid:       0,
		},
		{
			name:      "vid 0 on cn2, not related to mgmt",
			returnErr: false,
			errKey:    "",
			linkname:  "cn2-br",
			vid:       0,
		},
		{
			name:      "vid 100 on cn2, not related to mgmt",
			returnErr: false,
			errKey:    "",
			linkname:  "cn2-br.100",
			vid:       0,
		},
		{
			name:      "vid 100 on mgmt",
			returnErr: false,
			errKey:    "",
			linkname:  "mgmt-br.100",
			vid:       100,
		},
		{
			name:      "vid 100 on mgmt, malformed",
			returnErr: true,
			errKey:    "invalid link name",
			linkname:  "mgmt-br.100.3", // should not occur in real environment
			vid:       0,
		},
		{
			name:      "vid 100a1 on mgmt, malformed",
			returnErr: true,
			errKey:    "cannot convert",
			linkname:  "mgmt-br.100a1", // should not occur in real environment
			vid:       0,
		},
		{
			name:      "vid unknown on mgmt, malformed",
			returnErr: true,
			errKey:    "cannot convert",
			linkname:  "mgmt-br.", // should not occur in real environment
			vid:       0,
		},
		{
			name:      "vid 0 on mgmt, return 0",
			returnErr: false,
			errKey:    "",
			linkname:  "mgmt-br.0", // should not occur in real environment
			vid:       0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			link := &netlink.Bridge{}
			link.LinkAttrs.Name = tc.linkname

			links := []netlink.Link{
				link,
			}

			vid, err := getMgmtVlan(links)
			assert.True(t, tc.returnErr == (err != nil))
			if tc.returnErr {
				if tc.errKey != "" {
					assert.True(t, strings.Contains(err.Error(), tc.errKey))
				}
				return
			}
			assert.True(t, vid == tc.vid)
		})
	}
}
