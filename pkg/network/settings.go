package network

import (
	"encoding/json"

	settingsv1 "github.com/rancher/harvester/pkg/apis/harvester.cattle.io/v1alpha1"
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
	_, err := settingClient.Get(networkSettingsName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			networkSetting := &NetworkSetting{}
			jsonNetwork, err := json.Marshal(networkSetting)
			if err != nil {
				return err
			}

			setting := &settingsv1.Setting{
				ObjectMeta: metav1.ObjectMeta{
					Name: networkSettingsName,
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
