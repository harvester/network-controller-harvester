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
			name:      "Test Basic feature of vlanidset",
			returnErr: true,
			errKey:    "the length of",
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

			vis.safelyUnSetVID(33)
			assert.True(t, vis.vlanCount == 5) // 1, 100, 101, 102, 105

			vis.safelyUnSetVID(102)
			assert.True(t, vis.vlanCount == 4) // 1, 100, 101, 105
		})
	}
}
