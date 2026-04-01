package utils

import (
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
)

const (
	KeyUnderlayIntf = "ovn.kubernetes.io/tunnel_interface"
)

func MatchNode(node *v1.Node, selector labels.Selector) (bool, error) {
	if node == nil {
		return false, fmt.Errorf("node not found")
	}

	if node.DeletionTimestamp != nil {
		return false, nil
	}

	if !selector.Matches(labels.Set(node.Labels)) {
		// node doesn't match the selector, skip processing
		return false, nil
	}

	return true, nil
}
