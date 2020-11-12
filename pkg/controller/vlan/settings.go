package vlan

import (
	"encoding/json"
	"fmt"

	harvesterv1 "github.com/rancher/harvester/pkg/apis/harvester.cattle.io/v1alpha1"
	harvcontroller "github.com/rancher/harvester/pkg/generated/controllers/harvester.cattle.io/v1alpha1"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type NetworkSetting struct {
	// physical NIC(network interface card)
	NIC string

	// previous configured NIC
	ConfiguredNIC string
}

func initNetworkSettings(settingClient harvcontroller.SettingClient) error {
	_, err := settingClient.Get(NetworkSettingName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			networkSetting := &NetworkSetting{}
			jsonNetwork, err := json.Marshal(networkSetting)
			if err != nil {
				return err
			}

			setting := &harvesterv1.Setting{
				ObjectMeta: metav1.ObjectMeta{
					Name: NetworkSettingName,
				},
				Default: string(jsonNetwork),
			}

			sett, err := settingClient.Create(setting)
			if err != nil {
				if errors.IsAlreadyExists(err) {
					logrus.Println("skip to create the default network setting as it is already exist")
					return nil
				}
				return err
			}
			logrus.Printf("success initialized network settings: %v", sett.Default)
			return nil
		}
		return err
	}
	return nil
}

func EncodeNetworkSettings(setting *NetworkSetting) (string, error) {
	bytes, err := json.Marshal(setting)
	if err != nil {
		return "", fmt.Errorf("marshal failed, error: %w, networkSetting: %+v", err, setting)
	}

	return string(bytes), nil
}

func DecodeNetworkSettings(value string) (*NetworkSetting, error) {
	setting := &NetworkSetting{}
	if err := json.Unmarshal([]byte(value), setting); err != nil {
		return nil, fmt.Errorf("unmarshal failed, error: %w, value: %s", err, value)
	}

	return setting, nil
}
