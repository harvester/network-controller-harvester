package nad

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-ping/ping"
	ctlcniv1 "github.com/harvester/harvester/pkg/generated/controllers/k8s.cni.cncf.io/v1"
	cniv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	ctlbatchv1 "github.com/rancher/wrangler/pkg/generated/controllers/batch/v1"
	"k8s.io/klog"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/harvester/harvester-network-controller/pkg/config"
	"github.com/harvester/harvester-network-controller/pkg/utils"
)

const (
	ControllerName = "harvester-network-manager-nad-controller"

	jobContainerName      = "network-helper"
	jobServiceAccountName = "harvester-network-helper"
	JobEnvNadNetwork = "NAD_NETWORKS"
	JobEnvDHCPServer      = "DHCP_SERVER"

	defaultInterface = "net1"

	defaultPingTimes   = 5
	defaultCheckPeriod = 15 * time.Minute
)

type nameWithNamespace struct {
	namespace string
	name      string
}

type checkMap struct {
	items map[nameWithNamespace]string
	mutex *sync.RWMutex
}

type Handler struct {
	namespace   string
	helperImage string

	jobClient ctlbatchv1.JobClient
	jobCache  ctlbatchv1.JobCache
	nadClient ctlcniv1.NetworkAttachmentDefinitionClient
	nadCache  ctlcniv1.NetworkAttachmentDefinitionCache

	*checkMap
}

func Register(ctx context.Context, management *config.Management) error {
	job := management.BatchFactory.Batch().V1().Job()
	nad := management.CniFactory.K8s().V1().NetworkAttachmentDefinition()

	handler := &Handler{
		namespace:   management.Options.Namespace,
		helperImage: management.Options.HelperImage,
		jobClient:   job,
		jobCache:    job.Cache(),
		nadClient:   nad,
		nadCache:    nad.Cache(),
		checkMap: &checkMap{
			items: make(map[nameWithNamespace]string),
			mutex: new(sync.RWMutex),
		},
	}

	go handler.CheckConnectivity()

	nad.OnChange(ctx, ControllerName, handler.OnChange)
	nad.OnRemove(ctx, ControllerName, handler.OnRemove)

	return nil
}

func (h Handler) OnChange(key string, nad *cniv1.NetworkAttachmentDefinition) (*cniv1.NetworkAttachmentDefinition, error) {
	if nad == nil || nad.DeletionTimestamp != nil {
		return nil, nil
	}

	// check annotations
	if nad.Annotations == nil || nad.Annotations[utils.KeyNetworkConf] == "" {
		return nad, nil
	}
	networkConf, err := utils.NewLayer3NetworkConf(nad.Annotations[utils.KeyNetworkConf])
	if err != nil {
		return nil, fmt.Errorf("invalid layer 3 network configure: %w", err)
	}

	if networkConf.CIDR != "" && networkConf.Gateway != "" {
		// set connectivity as the initial status unknown
		if networkConf.Connectivity == "" {
			if err := h.setUnknown(nad, networkConf); err != nil {
				return nil, err
			}
		}
		// add item to map
		h.addItem(nad.Namespace, nad.Name, networkConf.Gateway)
		return nad, nil
	}

	if networkConf.Mode != utils.Auto {
		return nad, nil
	}
	// create or update job to get layer 3 network information automatically
	if err := h.createOrUpdateJob(nad, networkConf.ServerIPAddr); err != nil {
		return nil, err
	}

	return nad, nil
}

func (h Handler) setUnknown(nad *cniv1.NetworkAttachmentDefinition, networkConf *utils.Layer3NetworkConf) error {
	networkConf.Connectivity = utils.Unknown
	nadCopy := nad.DeepCopy()
	nadStr, err := networkConf.ToString()
	if err != nil {
		return err
	}
	nad.Annotations[utils.KeyNetworkConf] = nadStr

	_, err = h.nadClient.Update(nadCopy)

	return err
}

func (h Handler) OnRemove(key string, nad *cniv1.NetworkAttachmentDefinition) (*cniv1.NetworkAttachmentDefinition, error) {
	if nad == nil {
		return nil, nil
	}

	name := nad.Namespace + "-" + nad.Name
	if _, err := h.jobCache.Get(h.namespace, name); err != nil && !apierrors.IsNotFound(err) {
		return nil, err
	} else if err == nil {
		if err := h.jobClient.Delete(h.namespace, name, &metav1.DeleteOptions{}); err != nil {
			return nil, err
		}
	}

	h.removeItem(nad.Namespace, nad.Name)

	return nad, nil
}

func (h Handler) createOrUpdateJob(nad *cniv1.NetworkAttachmentDefinition, dhcpServerAddr string) error {
	job, err := h.jobCache.Get(h.namespace, nad.Namespace+"-"+nad.Name)
	if err != nil && !errors.IsNotFound(err) {
		return err
	} else if err == nil {
		// update job
		job, err = constructJob(job, h.namespace, h.helperImage, dhcpServerAddr, nad)
		if err != nil {
			return err
		}
		if _, err := h.jobClient.Update(job); err != nil {
			return err
		}
	} else {
		// create job
		job, err = constructJob(nil, h.namespace, h.helperImage, dhcpServerAddr, nad)
		if err != nil {
			return err
		}
		if _, err := h.jobClient.Create(job); err != nil {
			return err
		}
	}

	return nil
}

func constructJob(cur *batchv1.Job, namespace, image, dhcpServerAddr string, nad *cniv1.NetworkAttachmentDefinition) (*batchv1.Job, error) {
	job := &batchv1.Job{}
	if cur != nil {
		job = cur.DeepCopy()
	} else {
		job.Name = nad.Namespace + "-" + nad.Name
		job.Namespace = namespace
	}

	selectedNetworks, err := utils.NadSelectedNetworks([]cniv1.NetworkSelectionElement{
		{
			InterfaceRequest: defaultInterface,
			Namespace:        nad.Namespace,
			Name:             nad.Name,
		},
	}).ToString()
	if err != nil {
		return nil, err
	}

	job.Spec.Template = corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				cniv1.NetworkAttachmentAnnot: selectedNetworks,
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  jobContainerName,
					Image: image,
					Env: []corev1.EnvVar{
						{
							Name: JobEnvNadNetwork,
							ValueFrom: &corev1.EnvVarSource{
								FieldRef: &corev1.ObjectFieldSelector{
									FieldPath: fmt.Sprintf("metadata.annotations['%s']", cniv1.NetworkAttachmentAnnot),
								},
							},
						},
						{
							Name: JobEnvDHCPServer,
							Value: dhcpServerAddr,
						},
					},
					ImagePullPolicy: corev1.PullIfNotPresent,
				},
			},
			RestartPolicy:      corev1.RestartPolicyNever,
			ServiceAccountName: jobServiceAccountName,
		},
	}

	return job, nil
}

func (h Handler) CheckConnectivity() {
	ticker := time.NewTicker(defaultCheckPeriod)

	for range ticker.C {
		h.mutex.RLock()
		for nn, gw := range h.items {
			go func(nn nameWithNamespace, gw string) {
				connectivity, err := pingGW(gw)
				if err != nil {
					klog.Error(err)
					return
				}
				if err := h.updateNADConnectivity(nn.namespace, nn.name, connectivity); err != nil {
					klog.Error(err)
					return
				}
			}(nn, gw)
		}
		h.mutex.RUnlock()
	}
}

func pingGW(gw string) (utils.Connectivity, error) {
	connectivity := utils.Unknown

	pinger, err := ping.NewPinger(gw)
	if err != nil {
		return connectivity, fmt.Errorf("create pinger failed, error: %s", err.Error())
	}
	pinger.SetPrivileged(true)
	pinger.Count = defaultPingTimes
	if err := pinger.Run(); err != nil {
		return connectivity, err
	} // blocks until finished
	stats := pinger.Statistics()

	if stats.PacketsSent != stats.PacketsRecv {
		connectivity = utils.Unconnetable
	} else {
		connectivity = utils.Connectable
	}

	return connectivity, nil
}

func (h Handler) updateNADConnectivity(namespace, name string, connectivity utils.Connectivity) error {
	nad, err := h.nadCache.Get(namespace, name)
	if err != nil {
		return fmt.Errorf("get cache of %s/%s failed, error: %s", namespace, name, err)
	}
	if nad.Annotations == nil || nad.Annotations[utils.KeyNetworkConf] == "" {
		return nil
	}
	networkConf, err := utils.NewLayer3NetworkConf(nad.Annotations[utils.KeyNetworkConf])
	if err != nil {
		return fmt.Errorf("invalid layer 3 network configure: %w", err)
	}

	if networkConf.Connectivity == connectivity {
		return nil
	}

	networkConf.Connectivity = connectivity
	nadCopy := nad.DeepCopy()
	confStr, err := networkConf.ToString()
	if err != nil {
		return err
	}
	nadCopy.Annotations[utils.KeyNetworkConf] = confStr
	if _, err := h.nadClient.Update(nadCopy); err != nil {
		return err
	}

	return nil
}

func (c *checkMap) addItem(namespace, name, addr string) {
	nn := nameWithNamespace{namespace: namespace, name: name}
	c.mutex.RLock()
	oldAddr, ok := c.items[nn]
	c.mutex.RUnlock()

	if !ok || oldAddr != addr {
		c.mutex.Lock()
		c.items[nn] = addr
		c.mutex.Unlock()
	}
}

func (c *checkMap) removeItem(namespace, name string) {
	nn := nameWithNamespace{namespace: namespace, name: name}
	c.mutex.RLock()
	_, ok := c.items[nn]
	c.mutex.RUnlock()

	if ok {
		c.mutex.Lock()
		delete(c.items, nn)
		c.mutex.Unlock()
	}
}
