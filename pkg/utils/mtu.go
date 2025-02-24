package utils

import (
	"fmt"
	"strconv"
)

func IsValidMTU(MTU int) bool {
	return MTU == 0 || (MTU >= MinMTU && MTU <= MaxMTU)
}

func IsDefaultMTU(MTU int) bool {
	return MTU == DefaultMTU
}

func AreEqualMTUs(MTU1, MTU2 int) bool {
	return (MTU1 == MTU2) || (MTU1 == 0 && MTU2 == DefaultMTU) || (MTU1 == DefaultMTU && MTU2 == 0)
}

func GetMTUFromAnnotation(annotation string) (int, error) {
	MTU, err := strconv.Atoi(annotation)
	if err != nil {
		return 0, fmt.Errorf("annotation %v value is not an integer, error %w", annotation, err)
	}
	if !IsValidMTU(MTU) {
		return 0, fmt.Errorf("annotation %v value is not in range [0, %v..%v]", annotation, MinMTU, MaxMTU)
	}
	return MTU, nil
}
