package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	testCnName    = "test-cn"
	testNadName   = "nad1"
	testNamespace = "test"
)

func TestNewVidSET(t *testing.T) {
	tests := []struct {
		name      string
		returnErr bool
		errKey    string
	}{
		{
			name:      "Test basic functions of vlanidset",
			returnErr: false,
			errKey:    "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			vis := NewVlanIDSet()
			assert.NotNil(t, vis)
			err := vis.SetVID(-1)
			assert.NotNil(t, err)
			err = vis.SetVID(4095)
			assert.NotNil(t, err)

			err = vis.SetVID(100)
			assert.Nil(t, err)
			err = vis.SetVID(101)
			assert.Nil(t, err)
			err = vis.SetVID(102)
			assert.Nil(t, err)
			assert.True(t, vis.vlanCount == 4) // 1, 100, 101, 102

			vis2 := NewVlanIDSet()
			err = vis2.SetVID(105)
			assert.Nil(t, err)
			err = vis2.SetVID(102)
			assert.Nil(t, err)
			assert.True(t, vis2.vlanCount == 3) // 1, 105, 102

			vis = vis.Append(vis2)
			assert.True(t, vis.vlanCount == 5) // 1, 100, 101, 102, 105
			assert.True(t, vis.VidSetToString() == "1,100,101,102,105")

			vis.safelyUnsetVID(33)
			assert.True(t, vis.vlanCount == 5) // 1, 100, 101, 102, 105

			vis.safelyUnsetVID(102)
			assert.True(t, vis.vlanCount == 4) // 1, 100, 101, 105

			assert.True(t, vis.VidSetToString() == "1,100,101,105")

			vis3 := NewVlanIDSet()
			vis4 := NewVlanIDSet()

			vis3.safelySetVIDCidr(5, "")
			vis3.safelySetVIDCidr(6, "")
			vis3.safelySetVIDCidr(7, "")
			assert.True(t, vis3.vlanCount == 4) // expect vids: 1,5,6,7

			vis4.safelySetVIDCidr(6, "")
			vis4.safelySetVIDCidr(7, "")
			vis4.safelySetVIDCidr(8, "")
			vis4.safelySetVIDCidr(9, "")
			assert.True(t, vis4.vlanCount == 5) // current vids: 1,6,7,8,9

			added, removed, err := vis3.Diff(vis4)
			assert.Nil(t, err)
			assert.True(t, added.GetVlanCount() == 1)   // add vid 5
			assert.True(t, removed.GetVlanCount() == 2) // remove vid 6,7
		})
	}
}
