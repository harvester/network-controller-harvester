package nad

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/go-ping/ping"
	cniv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	ctlbatchv1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/batch/v1"
	"github.com/tidwall/sjson"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io"
	networkv1 "github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io/v1beta1"
	"github.com/harvester/harvester-network-controller/pkg/config"
	ctlcniv1 "github.com/harvester/harvester-network-controller/pkg/generated/controllers/k8s.cni.cncf.io/v1"
	ctlnetworkv1 "github.com/harvester/harvester-network-controller/pkg/generated/controllers/network.harvesterhci.io/v1beta1"
	"github.com/harvester/harvester-network-controller/pkg/utils"
)

const (
	ControllerName        = "harvester-network-manager-nad-controller"
	DeprecatedFinalizer   = "wrangler.cattle.io/harvester-network-nad-controller"
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
	cns.OnChange(ctx, ControllerName, handler.OnCNChange)
	return nil
}

// Sync cluster network MTU value to all attached NADs
func (h Handler) OnCNChange(_ string, cn *networkv1.ClusterNetwork) (*networkv1.ClusterNetwork, error) {
	if cn == nil || cn.DeletionTimestamp != nil {
		return nil, nil
	}

	// MTU annotation is not set
	curMTU := cn.Annotations[utils.KeyUplinkMTU]
	if curMTU == "" {
		return nil, nil
	}

	MTU, err := utils.GetMTUFromString(curMTU)
	// skip if MTU is invalid
	if err != nil {
		logrus.Infof("cluster network %v has MTU annotation %v/%v with invalid value, skip to sync with nad %s", cn.Name, utils.KeyUplinkMTU, curMTU, err.Error())
		return nil, nil
	}

	nads, err := h.nadCache.List("", labels.Set(map[string]string{
		utils.KeyClusterNetworkLabel: cn.Name,
	}).AsSelector())
	if err != nil {
		return nil, fmt.Errorf("failed to list cluster network %v related nads, error %w", cn.Name, err)
	}

	// sync with the possible new MTU
	for _, nad := range nads {
		netConf := &utils.NetConf{}
		if err := json.Unmarshal([]byte(nad.Spec.Config), netConf); err != nil {
			return nil, fmt.Errorf("failed to Unmarshal nad %v config %v error %w", nad.Name, nad.Spec.Config, err)
		}

		if utils.AreEqualMTUs(MTU, netConf.MTU) {
			continue
		}

		// Don't modify the unmarshalled structure and marshal it again because some fields may be lost during unmarshalling.
		newConfig, err := sjson.Set(nad.Spec.Config, "mtu", MTU)
		if err != nil {
			return nil, fmt.Errorf("failed to set nad %v with new MTU %v error %w", nad.Name, MTU, err)
		}
		nadCopy := nad.DeepCopy()
		nadCopy.Spec.Config = newConfig
		if _, err := h.nadClient.Update(nadCopy); err != nil {
			return nil, err
		}
		logrus.Infof("sync cluster network %v annotation mtu %v/%v to nad %v", cn.Name, utils.KeyUplinkMTU, curMTU, nad.Name)
	}

	return nil, nil
}

// nad manager controller ensures all labels and sync cn
func (h Handler) OnChange(_ string, nad *cniv1.NetworkAttachmentDefinition) (*cniv1.NetworkAttachmentDefinition, error) {
	if nad == nil || nad.DeletionTimestamp != nil {
		return nil, nil
	}

	logrus.Infof("nad configuration %s/%s has been changed: %s", nad.Namespace, nad.Name, nad.Spec.Config)

	netconf, updated, err := h.ensureLabels(nad)
	if err != nil {
		return nil, fmt.Errorf("ensure labels of nad %s/%s failed, error: %w", nad.Namespace, nad.Name, err)
	}
	if updated {
		// nad labels is updated by controller (rarely happens, it should be done via mutator)
		// following IsVlanNad depends on those labels
		// wait until it is updated and then go to below
		return nad, nil
	}

	// overlay nad does not trigger following steps
	if utils.IsOverlayNad(nad) {
		return nad, nil
	}

	// the nad is not a vlan nad any more, always clear the job
	if !utils.IsVlanNad(nad) {
		if err := h.clearJob(nad); err != nil {
			return nil, err
		}
	} else {
		if err := h.EnsureJob2GetLayer3NetworkInfo(nad, netconf); err != nil {
			return nil, err
		}
	}
	// nad change triggers the re-compute of cn's vlanset
	if err := h.UpdateClusterNetworkVlanSet(nad); err != nil {
		return nil, err
	}

	return nad, nil
}

func (h Handler) OnRemove(_ string, nad *cniv1.NetworkAttachmentDefinition) (*cniv1.NetworkAttachmentDefinition, error) {
	if nad == nil {
		return nil, nil
	}

	logrus.Infof("nad configuration %s/%s has been removed", nad.Namespace, nad.Name)

	// overlay nad does not trigger following steps
	if utils.IsOverlayNad(nad) {
		return nad, nil
	}

	// always clear the job
	if err := h.clearJob(nad); err != nil {
		return nil, err
	}

	// nad change triggers the re-compute of cn's vlanset
	// due to the existing of trunk mode nad, deleting any nad might not cause changes on the birdge's vlan
	if err := h.UpdateClusterNetworkVlanSet(nad); err != nil {
		return nil, err
	}

	nadCopy := nad.DeepCopy()
	// check and remove DeprecatedFinalizer which was added by NAD controller available in agent
	updated := controllerutil.RemoveFinalizer(nadCopy, DeprecatedFinalizer)
	if updated {
		return h.nadClient.Update(nadCopy)
	}

	return nad, nil
}

// if nad is updated, return true
func (h Handler) ensureLabels(nad *cniv1.NetworkAttachmentDefinition) (*utils.NetConf, bool, error) {
	// always recheck the labels to ensure they are correct
	nadCopy := nad.DeepCopy()
	if nadCopy.Labels == nil {
		nadCopy.Labels = make(map[string]string)
	}

	netconf, err := utils.DecodeNadConfigToNetConf(nadCopy)
	if err != nil {
		return nil, false, err
	}
	cnName, err := netconf.GetClusterNetworkName()
	if err != nil {
		return nil, false, err
	}
	err = netconf.SetNetworkInfoToLabels(nadCopy.Labels)
	if err != nil {
		return nil, false, err
	}
	if cn, err := h.cnCache.Get(cnName); err != nil {
		return nil, false, err
	} else if networkv1.Ready.IsTrue(cn.Status) {
		utils.SetNadLabel(nadCopy, utils.KeyNetworkReady, utils.ValueTrue)
	} else {
		utils.SetNadLabel(nadCopy, utils.KeyNetworkReady, utils.ValueFalse)
	}
	if reflect.DeepEqual(nad.Labels, nadCopy.Labels) {
		return netconf, false, nil
	}
	if _, err := h.nadClient.Update(nadCopy); err != nil {
		return nil, false, err
	}

	return netconf, true, nil
}

func (h Handler) UpdateClusterNetworkVlanSet(nad *cniv1.NetworkAttachmentDefinition) error {
	// the caller has ensured this nad !IsOverlayNad
	cnname := utils.GetNadLabel(nad, utils.KeyClusterNetworkLabel)
	if cnname == "" {
		return nil
	}
	cn, err := h.cnCache.Get(cnname)
	if err != nil {
		// retry when cn is not found
		return err
	}
	if cn.DeletionTimestamp != nil {
		return nil
	}

	vids, err := utils.GeVlanIDSetFromClusterNetwork(cn.Name, h.nadCache)
	if err != nil {
		logrus.Infof("cluster network %s failed to get vlanset %s", cn.Name, err.Error())
		return err
	}
	vidstr, vidhash := vids.VidSetToStringHash()
	// no change
	if utils.AreClusterNetworkVlanAnnotationsUnchanged(cn, vidstr, vidhash) {
		return nil
	}
	logrus.Infof("update cn %v annotations %v:%v", cnname, utils.KeyVlanIDSetStrHash, vidhash)
	// update new vid and hash to cluster network
	cnCopy := cn.DeepCopy()
	utils.SetClusterNetworkVlanAnnotations(cnCopy, vidstr, vidhash)
	if _, err := h.cnClient.Update(cnCopy); err != nil {
		return fmt.Errorf("failed to update cluster network %s label %s/%s error %w", cnname, utils.KeyVlanIDSetStrHash, vidhash, err)
	}

	return nil
}

func (h Handler) EnsureJob2GetLayer3NetworkInfo(nad *cniv1.NetworkAttachmentDefinition, netconf *utils.NetConf) error {
	if utils.IsOverlayNad(nad) {
		return nil
	}

	networkConf := &utils.Layer3NetworkConf{}
	routeStr := utils.GetNadAnnotation(nad, utils.KeyNetworkRoute)
	if routeStr == "" {
		// no l3 label on nad like storage-network
		return nil
	}

	var err error
	networkConf, err = utils.NewLayer3NetworkConf(routeStr)
	if err != nil {
		return fmt.Errorf("invalid layer 3 network configure %v: %w", routeStr, err)
	}

	logrus.Infof("EnsureJob2GetLayer3NetworkInfo netconf: %+v", networkConf)

	// when nad's vlan is changed, the nad is marked as outdated
	// the outdated is updated by the following created new job
	// with the vlan label on job, it does not rely on nad's outdated flag to make decision
	if networkConf.Mode == utils.Auto {
		if err := h.clearOutdatedJob(nad, netconf, networkConf); err != nil {
			return err
		}
	} else {
		// manual mode does not need job, e.g. one nad only changes the route from auto to static
		if err := h.clearJob(nad); err != nil {
			return err
		}
	}

	if networkConf.CIDR != "" && networkConf.Gateway != "" && !networkConf.Outdated {
		// initialize connectivity
		if networkConf.Connectivity == "" {
			if err := h.initializeConnectivity(nad, networkConf); err != nil {
				logrus.Errorf("initialize connectivity of nad %s/%s failed, error: %v", nad.Namespace, nad.Name, err)
			} else {
				logrus.Infof("initialize connectivity of nad %s/%s successfully", nad.Namespace, nad.Name)
			}
		}
		h.addItem(nad.Namespace, nad.Name, networkConf.Gateway)
		return nil
	}

	if networkConf.Mode != utils.Auto {
		return nil
	}
	// create or update job to get layer 3 network information automatically, the `Outdated` will be updated by the job
	return h.createOrUpdateJob(nad, netconf, networkConf)
}

// always clear the job
func (h Handler) clearJob(nad *cniv1.NetworkAttachmentDefinition) error {
	name := utils.Name(nad.Namespace, nad.Name)
	if job, err := h.jobCache.Get(h.namespace, name); err != nil && !apierrors.IsNotFound(err) {
		return err
	} else if err == nil {
		// already onDelete, wait
		if job.DeletionTimestamp != nil {
			h.removeItem(nad.Namespace, nad.Name)
			return nil
		}

		logrus.Infof("Clear nad helper job %v, vid %v", name, job.Labels[utils.KeyVlanLabel])
		propagationPolicy := metav1.DeletePropagationBackground
		if err := h.jobClient.Delete(h.namespace, name, &metav1.DeleteOptions{
			PropagationPolicy: &propagationPolicy,
		}); err != nil {
			return err
		}

		h.removeItem(nad.Namespace, nad.Name)
		return nil
	}

	// IsNotFound, remove item from list anyway
	h.removeItem(nad.Namespace, nad.Name)

	return nil
}

// when netconf is set, it compares the job label to avoid deleting the target job
func (h Handler) clearOutdatedJob(nad *cniv1.NetworkAttachmentDefinition, netconf *utils.NetConf, l3netconf *utils.Layer3NetworkConf) error {
	name := utils.Name(nad.Namespace, nad.Name)
	if job, err := h.jobCache.Get(h.namespace, name); err != nil && !apierrors.IsNotFound(err) {
		return err
	} else if err == nil {
		// job is onDelete, wait
		if job.DeletionTimestamp != nil {
			h.removeItem(nad.Namespace, nad.Name)
			return nil
		}

		// without a label check, new job for the working nad might be cleared
		// when vlan id or dhcp server are changed, the job needs to be re-created
		if utils.AreJobLabelsDHCPInfoUnchanged(job.Labels, netconf, l3netconf) {
			return nil
		}

		logrus.Infof("Clear nad helper job %v, vid %v", name, job.Labels[utils.KeyVlanLabel])
		propagationPolicy := metav1.DeletePropagationBackground
		if err := h.jobClient.Delete(h.namespace, name, &metav1.DeleteOptions{
			PropagationPolicy: &propagationPolicy,
		}); err != nil && !apierrors.IsNotFound(err) {
			return err
		}

		h.removeItem(nad.Namespace, nad.Name)
		return nil
	}

	// IsNotFound, remove item from list anyway
	h.removeItem(nad.Namespace, nad.Name)

	return nil
}

func (h Handler) createOrUpdateJob(nad *cniv1.NetworkAttachmentDefinition, l2netconf *utils.NetConf, l3netconf *utils.Layer3NetworkConf) error {
	name := utils.Name(nad.Namespace, nad.Name)
	job, err := h.jobCache.Get(h.namespace, name)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}

		// create job
		job, err = constructJob(nil, h.namespace, h.helperImage, nad, l2netconf, l3netconf)
		if err != nil {
			return err
		}
		if _, err := h.jobClient.Create(job); err != nil {
			return err
		}
		logrus.Infof("Created nad helper job %v, dhcpServer %v, vid %v", job.Name, l3netconf.GetDHCPServerIPAddr(), l2netconf.GetVlanString())
	}

	// in case the nad's vlan is changed from e.g. 100 to 200, the old job is deleted and new job is created
	// but the job is using same name, need to ensure the sequences of them
	if job.DeletionTimestamp != nil {
		return fmt.Errorf("old job %v is on deleting, wait until it is gone and then create new job", job.Name)
	}

	// is already existing, update if some fields are invalid
	jobCopy, err := constructJob(job, h.namespace, h.helperImage, nad, l2netconf, l3netconf)
	if err != nil {
		return err
	}
	if equality.Semantic.DeepEqual(job, jobCopy) {
		return nil
	}
	if _, err := h.jobClient.Update(jobCopy); err != nil {
		return err
	}

	// normally, this should not happen
	logrus.Infof("Updated nad helper job %v, dhcpServer %v, vid %v", jobCopy.Name, l3netconf.GetDHCPServerIPAddr(), l2netconf.GetVlanString())

	return nil
}

func constructJob(cur *batchv1.Job, namespace, image string, nad *cniv1.NetworkAttachmentDefinition, netconf *utils.NetConf, l3netconf *utils.Layer3NetworkConf) (*batchv1.Job, error) {
	job := &batchv1.Job{}
	if cur != nil {
		job = cur.DeepCopy()
	} else {
		job.Name = utils.Name(nad.Namespace, nad.Name)
		job.Namespace = namespace
	}

	if netconf != nil {
		// add vlan label for future check
		if job.Labels == nil {
			job.Labels = make(map[string]string)
		}
		utils.SetDHCPInfo2JobLabels(job.Labels, netconf, l3netconf)
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
					Value: l3netconf.GetDHCPServerIPAddr(),
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
								Key:      network.GroupName + "/" + utils.GetNadLabel(nad, utils.KeyClusterNetworkLabel),
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
					logrus.Error(err)
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
	if nad.Annotations != nil && nad.Annotations[utils.KeyNetworkRoute] != "" {
		networkConf, err = utils.NewLayer3NetworkConf(nad.Annotations[utils.KeyNetworkRoute])
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
	nadCopy.Annotations[utils.KeyNetworkRoute] = confStr
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
