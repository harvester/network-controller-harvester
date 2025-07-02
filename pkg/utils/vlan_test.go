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
			assert.True(t, vis.GetVlanCount() == 3) // 100, 101, 102

			vis2 := NewVlanIDSet()
			err = vis2.SetVID(105)
			assert.Nil(t, err)
			err = vis2.SetVID(102)
			assert.Nil(t, err)
			assert.True(t, vis2.GetVlanCount() == 2) // 105, 102

			err = vis.Append(vis2)
			assert.Nil(t, err)
			assert.True(t, vis.GetVlanCount() == 4) // 100, 101, 102, 105
			assert.True(t, vis.VidSetToString() == "100,101,102,105")

			vis._unsetVID(33)
			assert.True(t, vis.GetVlanCount() == 4) // 100, 101, 102, 105

			vis._unsetVID(102)
			assert.True(t, vis.GetVlanCount() == 3) // 100, 101, 105

			assert.True(t, vis.VidSetToString() == "100,101,105")

			vis3 := NewVlanIDSet()
			vis4 := NewVlanIDSet()

			vis3._setVID(5)
			vis3._setVID(6)
			vis3._setVID(7)
			assert.True(t, vis3.vlanCount == 3) // expect vids:5,6,7

			vis4._setVID(6)
			vis4._setVID(7)
			vis4._setVID(8)
			vis4._setVID(9)
			assert.True(t, vis4.vlanCount == 4) // existing vids: 6,7,8,9

			added, removed, err := vis3.Diff(vis4) // run diff to get the to-add and to-remove list
			assert.Nil(t, err)
			assert.True(t, added.GetVlanCount() == 1)   // to add vid: 5
			assert.True(t, removed.GetVlanCount() == 2) // to remove vid: 8,9

			// test the untag, l2 tag vid operations
			vis5, err := NewVlanIDSetFromSingleVID(0) // ungtag vlan
			assert.Nil(t, err)
			assert.True(t, vis5.GetVlanCount() == 0) // vid count 0
			assert.True(t, vis5.VidSetToString() == "")

			vis6, err := NewVlanIDSetFromSingleVID(111) // vlan 111
			assert.Nil(t, err)
			assert.True(t, vis6.GetVlanCount() == 1) // vid count 1
			assert.True(t, vis6.VidSetToString() == "111")

			vis7 := NewVlanIDSet()
			err = vis7.Append(vis5) // untag can be appended
			assert.Nil(t, err)

			assert.True(t, vis7.GetVlanCount() == 0) // after appending the untag
			assert.True(t, vis7.VidSetToString() == "")
			err = vis7.Append(vis6)
			assert.Nil(t, err)
			assert.True(t, vis7.GetVlanCount() == 1) // after appending the tag
			assert.True(t, vis7.VidSetToString() == "111")
			err = vis7.SetVID(120) // set another vid 120
			assert.Nil(t, err)
			assert.True(t, vis7.GetVlanCount() == 2) // vid count 2: 111, 120
			assert.True(t, vis7.VidSetToString() == "111,120")

			// single vid mode can't append others
			err = vis5.Append(vis6)
			assert.NotNil(t, err)

			// diff is only fit for trunk mode
			_, _, err = vis5.Diff(vis6)
			assert.NotNil(t, err)
		})
	}
}
