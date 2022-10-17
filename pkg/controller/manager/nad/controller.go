package nad

import (
	"context"
	"fmt"
	"github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io"
	"sync"
	"time"

	"github.com/go-ping/ping"
	ctlcniv1 "github.com/harvester/harvester/pkg/generated/controllers/k8s.cni.cncf.io/v1"
	cniv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	ctlbatchv1 "github.com/rancher/wrangler/pkg/generated/controllers/batch/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"

	"github.com/harvester/harvester-network-controller/pkg/config"
	ctlnetworkv1 "github.com/harvester/harvester-network-controller/pkg/generated/controllers/network.harvesterhci.io/v1beta1"
	"github.com/harvester/harvester-network-controller/pkg/utils"
)

const (
	ControllerName = "harvester-network-manager-nad-controller"

	jobContainerName      = "network-helper"
	jobServiceAccountName = "harvester-network-helper"
	JobEnvNadNetwork      = "NAD_NETWORKS"
	JobEnvDHCPServer      = "DHCP_SERVER"

	defaultInterface = "net1"

	defaultPingTimes            = 5
	defaultPingTimeout          = 10 * time.Second
	defaultCheckPeriod          = 15 * time.Minute
	defaultAllowPackageLostRate = 20
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
	cnClient  ctlnetworkv1.ClusterNetworkClient
	cnCache   ctlnetworkv1.ClusterNetworkCache

	*checkMap
}

func Register(ctx context.Context, management *config.Management) error {
	jobs := management.BatchFactory.Batch().V1().Job()
	nads := management.CniFactory.K8s().V1().NetworkAttachmentDefinition()
	cns := management.HarvesterNetworkFactory.Network().V1beta1().ClusterNetwork()

	handler := &Handler{
		namespace:   management.Options.Namespace,
		helperImage: management.Options.HelperImage,
		jobClient:   jobs,
		jobCache:    jobs.Cache(),
		nadClient:   nads,
		nadCache:    nads.Cache(),
		cnClient:    cns,
		cnCache:     cns.Cache(),
		checkMap: &checkMap{
			items: make(map[nameWithNamespace]string),
			mutex: new(sync.RWMutex),
		},
	}

	go handler.CheckConnectivityPeriodically()

	nads.OnChange(ctx, ControllerName, handler.OnChange)
	nads.OnRemove(ctx, ControllerName, handler.OnRemove)

	return nil
}

func (h Handler) OnChange(key string, nad *cniv1.NetworkAttachmentDefinition) (*cniv1.NetworkAttachmentDefinition, error) {
	if nad == nil || nad.DeletionTimestamp != nil {
		return nil, nil
	}

	if utils.IsEmptyNAD(nad) {
		return nad, nil
	}

	klog.Infof("nad configuration %s has been changed: %s", nad.Name, nad.Spec.Config)

	if err := h.EnsureJob2GetLayer3NetworkInfo(nad); err != nil {
		return nil, err
	}

	return nad, nil
}

func (h Handler) OnRemove(key string, nad *cniv1.NetworkAttachmentDefinition) (*cniv1.NetworkAttachmentDefinition, error) {
	if nad == nil {
		return nil, nil
	}

	if utils.IsEmptyNAD(nad) {
		return nad, nil
	}

	if err := h.ClearJob(nad); err != nil {
		return nil, err
	}

	return nad, nil
}

func (h Handler) EnsureJob2GetLayer3NetworkInfo(nad *cniv1.NetworkAttachmentDefinition) error {
	networkConf := &utils.Layer3NetworkConf{}
	if nad.Annotations != nil && nad.Annotations[utils.KeyNetworkConf] != "" {
		var err error
		networkConf, err = utils.NewLayer3NetworkConf(nad.Annotations[utils.KeyNetworkConf])
		if err != nil {
			return fmt.Errorf("invalid layer 3 network configure: %w", err)
		}
	}
	klog.Infof("netconf: %+v", networkConf)

	if networkConf.CIDR != "" && networkConf.Gateway != "" {
		// initialize connectivity
		if networkConf.Connectivity == "" {
			if err := h.initializeConnectivity(nad, networkConf); err != nil {
				klog.Errorf("initialize connectivity of nad %s/%s failed, error: %v", nad.Namespace, nad.Name, err)
			} else {
				klog.Infof("initialize connectivity of nad %s/%s successfully", nad.Namespace, nad.Name)
			}
		}
		// add item to map
		h.addItem(nad.Namespace, nad.Name, networkConf.Gateway)
		return nil
	}

	if networkConf.Mode != utils.Auto {
		return nil
	}
	// create or update job to get layer 3 network information automatically
	if err := h.createOrUpdateJob(nad, networkConf.ServerIPAddr); err != nil {
		return err
	}

	return nil
}

func (h Handler) ClearJob(nad *cniv1.NetworkAttachmentDefinition) error {
	name := nad.Namespace + "-" + nad.Name
	if _, err := h.jobCache.Get(h.namespace, name); err != nil && !apierrors.IsNotFound(err) {
		return err
	} else if err == nil {
		if err := h.jobClient.Delete(h.namespace, name, &metav1.DeleteOptions{}); err != nil {
			return err
		}
	}
	h.removeItem(nad.Namespace, nad.Name)
	return nil
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

	// annotations
	if job.Spec.Template.ObjectMeta.Annotations == nil {
		job.Spec.Template.ObjectMeta.Annotations = make(map[string]string)
	}
	job.Spec.Template.ObjectMeta.Annotations[cniv1.NetworkAttachmentAnnot] = selectedNetworks

	// podSpec
	job.Spec.Template.Spec.Containers = []corev1.Container{
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
					Name:  JobEnvDHCPServer,
					Value: dhcpServerAddr,
				},
			},
			ImagePullPolicy: corev1.PullIfNotPresent,
		},
	}
	// Add nodeAffinity to prove the job pod is scheduled to the proper node with the specified cluster network
	job.Spec.Template.Spec.Affinity = &corev1.Affinity{
		NodeAffinity: &corev1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
				NodeSelectorTerms: []corev1.NodeSelectorTerm{
					{
						MatchExpressions: []corev1.NodeSelectorRequirement{
							{
								Key:      network.GroupName + "/" + nad.Labels[utils.KeyClusterNetworkLabel],
								Operator: corev1.NodeSelectorOpIn,
								Values: []string{
									utils.ValueTrue,
								},
							},
						},
					},
				},
			},
		},
	}
	job.Spec.Template.Spec.RestartPolicy = corev1.RestartPolicyNever
	job.Spec.Template.Spec.ServiceAccountName = jobServiceAccountName
	backoffLimit := int32(1)
	job.Spec.BackoffLimit = &backoffLimit

	return job, nil
}

func (h Handler) CheckConnectivityPeriodically() {
	ticker := time.NewTicker(defaultCheckPeriod)

	for range ticker.C {
		h.mutex.RLock()
		for nn, gw := range h.items {
			go func(nn nameWithNamespace, gw string) {
				if err := h.checkConnectivity(nn.namespace, nn.name, gw); err != nil {
					klog.Error(err)
					return
				}
			}(nn, gw)
		}
		h.mutex.RUnlock()
	}
}

func (h Handler) checkConnectivity(namespace, name, gw string) error {
	connectivity, err := pingGW(gw)
	if err != nil {
		return err
	}

	nad, err := h.nadCache.Get(namespace, name)
	if err != nil {
		return fmt.Errorf("get cache of %s/%s failed, error: %s", namespace, name, err)
	}

	networkConf := &utils.Layer3NetworkConf{}
	if nad.Annotations != nil && nad.Annotations[utils.KeyNetworkConf] != "" {
		networkConf, err = utils.NewLayer3NetworkConf(nad.Annotations[utils.KeyNetworkConf])
		if err != nil {
			return fmt.Errorf("invalid layer 3 network configure: %w", err)
		}
	}

	if networkConf.Connectivity == connectivity {
		return nil
	}
	networkConf.Connectivity = connectivity

	return h.updateNetworkConf(nad, networkConf)
}

func (h Handler) initializeConnectivity(nad *cniv1.NetworkAttachmentDefinition, networkConf *utils.Layer3NetworkConf) error {
	connectivity, err := pingGW(networkConf.Gateway)
	if err != nil {
		return err
	}

	if networkConf.Connectivity == connectivity {
		return nil
	}
	networkConf.Connectivity = connectivity

	return h.updateNetworkConf(nad, networkConf)
}

func pingGW(gw string) (utils.Connectivity, error) {
	connectivity := utils.PingFailed

	pinger, err := ping.NewPinger(gw)
	if err != nil {
		return connectivity, fmt.Errorf("create pinger failed, error: %s", err.Error())
	}
	pinger.SetPrivileged(true)
	pinger.Count = defaultPingTimes
	pinger.Timeout = defaultPingTimeout
	if err := pinger.Run(); err != nil {
		return connectivity, fmt.Errorf("ping gw %s failed, error: %w", gw, err)
	} // blocks until finished
	stats := pinger.Statistics()

	if stats.PacketLoss > defaultAllowPackageLostRate {
		connectivity = utils.Unconnectable
	} else {
		connectivity = utils.Connectable
	}

	return connectivity, nil
}

func (h Handler) updateNetworkConf(nad *cniv1.NetworkAttachmentDefinition, networkConf *utils.Layer3NetworkConf) error {
	nadCopy := nad.DeepCopy()
	confStr, err := networkConf.ToString()
	if err != nil {
		return err
	}
	if nadCopy.Annotations == nil {
		nadCopy.Annotations = make(map[string]string)
	}
	nadCopy.Annotations[utils.KeyNetworkConf] = confStr
	if _, err := h.nadClient.Update(nadCopy); err != nil {
		return fmt.Errorf("update nad %s/%s failed, error: %w", nad.Namespace, nad.Name, err)
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
