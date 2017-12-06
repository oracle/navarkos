/*
Copyright 2017 The Kubernetes Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package deployment

import (
	"fmt"
	"sort"
	"strconv"
	"time"

	apiv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/flowcontrol"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/kubernetes/federation/pkg/federation-controller/util"
	"k8s.io/kubernetes/pkg/controller"

	"github.com/golang/glog"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"

	common "github.com/kubernetes-incubator/navarkos/pkg/common"
	cluster "github.com/kubernetes-incubator/navarkos/pkg/controller/cluster"
	extensionsv1 "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1beta1 "k8s.io/kubernetes/federation/apis/federation/v1beta1"
	federationclientset "k8s.io/kubernetes/federation/client/clientset_generated/federation_clientset"
	fedtypes "k8s.io/kubernetes/federation/pkg/federatedtypes"
	"k8s.io/kubernetes/federation/pkg/federation-controller/util/clusterselector"
)

const (
	allClustersKey = "ALL_CLUSTERS"
)

//DeploymentController provides for distribution of deployment
type DeploymentController struct {

	// For triggering reconciliation of a single deployment. This is
	// used when there is an add/update/delete operation on a resource
	deliverer *util.DelayingDeliverer

	// For triggering reconciliation of all target resources. This is
	// used when a new cluster becomes available.
	clusterDeliverer *util.DelayingDeliverer

	// Contains resources present in members of federation.
	informer util.FederatedInformer
	// Definitions of resources that should be federated.
	store cache.Store
	// Informer controller for resources that should be federated.
	controller cache.Controller

	// Work queue allowing parallel processing of resources
	workQueue workqueue.Interface

	// federationClient used to operate cluster
	federationClient federationclientset.Interface

	// Backoff manager
	backoff *flowcontrol.Backoff

	reviewDelay           time.Duration
	clusterAvailableDelay time.Duration

	//navarkos cluster controller
	cController cluster.ClusterController
}

// StartDeploymentController starts a new deployment controller
func StartDeploymentController(cController *cluster.ClusterController, config *restclient.Config, stopChan <-chan struct{}, minimizeLatency bool) {
	restclient.AddUserAgent(config, "navarkos-deployment-controller")
	client := federationclientset.NewForConfigOrDie(config)
	controller := newDeploymentController(client, cController)
	cController.OnClusterChange = controller.reconcileOnClusterChange
	if minimizeLatency {
		controller.minimizeLatency()
	}
	glog.Infof("Starting navarkos deployment controller")
	controller.Run(stopChan)
}

func newDeploymentController(client federationclientset.Interface, cController *cluster.ClusterController) *DeploymentController {

	dc := &DeploymentController{
		reviewDelay:           time.Second * 10,
		clusterAvailableDelay: time.Second * 10,
		workQueue:             workqueue.New(),
		federationClient:      client,
		backoff:               flowcontrol.NewBackOff(5*time.Second, time.Minute),
		cController:           *cController,
	}

	// Build delivereres for triggering reconciliations.
	dc.deliverer = util.NewDelayingDeliverer()
	dc.clusterDeliverer = util.NewDelayingDeliverer()

	// Start informer in federated API servers on the resource type that should be federated.
	dc.store, dc.controller = cache.NewInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return dc.federationClient.ExtensionsV1beta1().Deployments(metav1.NamespaceAll).List(options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return dc.federationClient.ExtensionsV1beta1().Deployments(metav1.NamespaceAll).Watch(options)
			},
		},
		&extensionsv1.Deployment{},
		controller.NoResyncPeriodFunc(),
		util.NewTriggerOnAllChanges(func(obj runtime.Object) { dc.deliverObj(obj, 0, false) }))

	return dc
}

// minimizeLatency reduces delays and timeouts to make the controller more responsive (useful for testing).
func (dc *DeploymentController) minimizeLatency() {
	dc.clusterAvailableDelay = 5 * time.Second
	//dc.clusterUnavailableDelay = time.Second
	dc.reviewDelay = 50 * time.Millisecond
	//dc.smallDelay = 20 * time.Millisecond
	//dc.updateTimeout = 5 * time.Second
}

func (dc *DeploymentController) deliverObj(obj runtime.Object, delay time.Duration, failed bool) {
	deployment := obj.(*extensionsv1.Deployment)
	qualifiedName := fedtypes.QualifiedName{Namespace: deployment.Namespace, Name: deployment.Name}
	dc.deliver(qualifiedName, delay, failed)
}

// Adds backoff to delay if this delivery is related to some failure. Resets backoff if there was no failure.
func (dc *DeploymentController) deliver(qualifiedName fedtypes.QualifiedName, delay time.Duration, failed bool) {
	key := qualifiedName.String()
	if failed {
		dc.backoff.Next(key, time.Now())
		delay = delay + dc.backoff.Get(key)
	} else {
		dc.backoff.Reset(key)
	}
	dc.deliverer.DeliverAfter(key, &qualifiedName, delay)
}

// Run begins watching and syncing.
func (dc *DeploymentController) Run(stopChan <-chan struct{}) {
	go dc.controller.Run(stopChan)
	dc.deliverer.StartWithHandler(func(item *util.DelayingDelivererItem) {
		dc.workQueue.Add(item)
	})

	dc.clusterDeliverer.StartWithHandler(func(_ *util.DelayingDelivererItem) {
		dc.reconcileClusters()
	})

	// TODO: Allow multiple workers.
	go wait.Until(dc.worker, time.Second, stopChan)

	util.StartBackoffGC(dc.backoff, stopChan)

	// Ensure all goroutines are cleaned up when the stop channel closes
	go func() {
		<-stopChan
		dc.workQueue.ShutDown()
		dc.deliverer.Stop()
		//dc.clusterDeliverer.Stop()
	}()
}

type reconciliationStatus int

const (
	statusAllOK reconciliationStatus = iota
	statusNeedsRecheck
	statusError
	statusNotSynced
	statusCapacityRequested
)

func (dc *DeploymentController) reconcileClusters() {

	status := dc.rescheduleAll()

	switch status {
	case statusAllOK:
		break
	case statusError:
		dc.clusterDeliverer.DeliverAt(allClustersKey, nil, time.Now().Add(dc.reviewDelay))
	case statusNeedsRecheck:
		dc.clusterDeliverer.DeliverAt(allClustersKey, nil, time.Now().Add(dc.reviewDelay))
	case statusNotSynced:
		dc.clusterDeliverer.DeliverAt(allClustersKey, nil, time.Now().Add(dc.clusterAvailableDelay))
	case statusCapacityRequested:
		dc.clusterDeliverer.DeliverAt(allClustersKey, nil, time.Now().Add(dc.clusterAvailableDelay))
	}
}

func (dc *DeploymentController) reconcileOnClusterChange() {
	dc.clusterDeliverer.DeliverAt(allClustersKey, nil, time.Now())
}

func (dc *DeploymentController) worker() {
	for {
		obj, quit := dc.workQueue.Get()
		if quit {
			return
		}

		item := obj.(*util.DelayingDelivererItem)
		qualifiedName := item.Value.(*fedtypes.QualifiedName)
		status := dc.reconcile(*qualifiedName)
		dc.workQueue.Done(item)

		switch status {
		case statusAllOK:
			break
		case statusError:
			dc.deliver(*qualifiedName, 0, true)
		case statusNeedsRecheck:
			dc.deliver(*qualifiedName, dc.reviewDelay, false)
		case statusNotSynced:
			dc.deliver(*qualifiedName, dc.clusterAvailableDelay, false)
		case statusCapacityRequested:
			dc.deliver(*qualifiedName, dc.clusterAvailableDelay, false)
		}
	}
}

func (dc *DeploymentController) objFromCache(key string) (runtime.Object, error) {
	cachedObj, exist, err := dc.store.GetByKey(key)
	if err != nil {
		wrappedErr := fmt.Errorf("Failed to query store for %q: %v", key, err)
		utilruntime.HandleError(wrappedErr)
		return nil, err
	}
	if !exist {
		return nil, nil
	}
	return cachedObj.(runtime.Object).DeepCopyObject(), nil
}

//just a stub for now
func (dc *DeploymentController) isSynced() bool {
	return dc.cController.IsSynced()
}

func (dc *DeploymentController) reconcile(qualifiedName fedtypes.QualifiedName) reconciliationStatus {

	if !dc.isSynced() {
		return statusNotSynced
	}

	key := qualifiedName.String()

	glog.V(4).Infof("Starting to reconcile %v", key)
	startTime := time.Now()
	defer glog.V(4).Infof("Finished reconciling %v (duration: %v)", key, time.Now().Sub(startTime))

	obj, err := dc.objFromCache(key)
	if err != nil {
		return statusError
	}
	if obj == nil {
		return statusAllOK
	}

	fedDeployment := obj.(*extensionsv1.Deployment)

	if fedDeployment.ObjectMeta.DeletionTimestamp != nil {
		glog.V(4).Infof("Deployment %v is marked for deletion, no action is needed", fedDeployment.Name)
		return statusAllOK
	}

	return dc.schedule(fedDeployment)

}

func (dc *DeploymentController) schedule(dpmt *extensionsv1.Deployment) reconciliationStatus {

	checkStatus, err := dc.checkClusters(dpmt)
	if err != nil {
		glog.Error(err)
		return checkStatus
	}

	if checkStatus == statusAllOK {
		schedStatus := dc.rescheduleAll()
		return schedStatus
	}
	return checkStatus
}

func (dc *DeploymentController) rescheduleAll() reconciliationStatus {

	glog.V(2).Infof("Starting global rescheduling...")
	status := statusAllOK

	var deployments []*extensionsv1.Deployment
	for _, obj := range dc.store.List() {
		deployments = append(deployments, obj.(*extensionsv1.Deployment))
	}

	readyClusters, err := dc.cController.GetReadyClusters()
	if err != nil {
		glog.Error(err)
		return statusError
	}

	for _, cluster := range readyClusters {
		if !dc.cController.IsCapacityDataPresent(cluster) {
			glog.V(4).Infof("Cluster %s does not have capcity data updated", cluster.Name)
			return statusNotSynced
		}
		if dc.cController.IsClusterScaling(cluster) {
			glog.V(4).Infof("Cluster %s is scaling", cluster.Name)
			return statusNotSynced
		}

	}

	dpmt2update, err := GetSchedule(deployments, readyClusters)
	if err != nil {
		glog.Error(err)
		return statusError
	}

	if dpmt2update != nil {
		for _, updDpmt := range dpmt2update {
			glog.V(2).Infof("Updating cluster preferences for deployment %v", updDpmt.Name)

			err := dc.updateDeployment(updDpmt)
			if err != nil {
				glog.Error(err)
				status = statusError
			}
		}
	}

	return status
}

func (dc *DeploymentController) updateDeployment(dpmt *extensionsv1.Deployment) error {
	_, err := dc.federationClient.ExtensionsV1beta1().Deployments(dpmt.Namespace).Update(dpmt)
	return err
}

// selectedClusters filters the provided clusters into two slices, one containing the clusters selected by selector and the other containing the rest of the provided clusters.
func (dc *DeploymentController) selectClusters(dpmt *extensionsv1.Deployment, clusters []*v1beta1.Cluster) ([]*v1beta1.Cluster, []*v1beta1.Cluster, error) {
	selectedClusters := []*v1beta1.Cluster{}
	unselectedClusters := []*v1beta1.Cluster{}

	for _, cluster := range clusters {
		send, err := clusterselector.SendToCluster(cluster.Labels, dpmt.Annotations)
		if err != nil {
			return nil, nil, err
		} else if !send {
			unselectedClusters = append(unselectedClusters, cluster)
		} else {
			selectedClusters = append(selectedClusters, cluster)
		}
	}
	return selectedClusters, unselectedClusters, nil
}

func (dc *DeploymentController) checkClusters(fedDeployment *extensionsv1.Deployment) (reconciliationStatus, error) {

	desiredPods := *fedDeployment.Spec.Replicas

	// lets figure out how much additional capicity is needed and can this be covered by ready clusters
	readyClusters, err := dc.cController.GetReadyClusters()
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("Failed to get ready cluster list: %v", err))
	}

	var dpmtUsedPods int32

	var podsAvailable int32

	selectedReadyClusters, _, err := dc.selectClusters(fedDeployment, readyClusters)

	for _, rCluster := range selectedReadyClusters {
		clustersStatsAreUpToDate := false
		if clusterCapacityPodsStr, ok := rCluster.Annotations[common.NavarkosClusterCapacityPodsKey]; ok {
			if clusterUsedPodsStr, ok := rCluster.Annotations[common.NavarkosClusterCapacityUsedPodsKey]; ok {
				if getSecondsSincLastTransitionTime(rCluster) > 30 {
					clusterCapacityPods, _ := strconv.ParseInt(clusterCapacityPodsStr, 10, 32)
					clusterUsedPods, _ := strconv.ParseInt(clusterUsedPodsStr, 10, 32)
					podsAvailable += int32(clusterCapacityPods - clusterUsedPods)
					clustersStatsAreUpToDate = true
					dc.cController.GetClusterClient(rCluster)
					clusterDmpt, err := dc.cController.GetClusterDeployment(rCluster, fedDeployment.Namespace, fedDeployment.Name)
					if clusterDmpt != nil && err == nil {
						dpmtUsedPods += clusterDmpt.Status.Replicas
					}
				}
			}
		}

		if !clustersStatsAreUpToDate {
			return statusNotSynced, nil
		}
	}

	podsNeded := desiredPods - dpmtUsedPods

	if (podsAvailable - podsNeded) >= 0 {
		glog.V(4).Infof("checkClusters Still have enough capacity within ready clusters: pods available - %d, pods desired - %d, pods used by deployment - %d, additional pods needed - %d, .", podsAvailable, desiredPods, dpmtUsedPods, podsNeded)
		return statusAllOK, nil
	}
	glog.V(4).Infof("checkClusters Ran out of capacity, will spin up a new cluster/s: pods available - %d, pods desired - %d, pods used by deployment - %d, additional pods needed - %d, .", podsAvailable, desiredPods, dpmtUsedPods, podsNeded)

	unreadyClusters, err := dc.cController.GetUnreadyClusters()
	selectedUnReadyClusters, unselectedUnReadyClusters, err := dc.selectClusters(fedDeployment, unreadyClusters)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("Failed to get unready cluster list: %v", err))
	}

	if len(selectedUnReadyClusters) == 0 {
		glog.V(4).Infof("checkClusters no selectable offline clusters left")
		return statusAllOK, nil
	}

	priorityClusters, err := getPriorityClusters(selectedUnReadyClusters)

	if err != nil {
		utilruntime.HandleError(fmt.Errorf("Failed to get unready cluster list: %v", err))
		return statusError, err
	}

	status := statusAllOK
	for _, pCluster := range priorityClusters {
		glog.V(4).Infof("checkClusters cluster from selectedUnReadyClusters %v", pCluster.Name)
		gasClusterState := pCluster.Annotations[common.NavarkosClusterStateKey]
		glog.Infof("checkClusters cluster %s has clusterstate of %v", pCluster.Name, gasClusterState)
		if gasClusterState == common.NavarkosClusterStateOffline {
			updCluster, err := dc.cController.GetCluster(pCluster.Name)
			if err == nil {
				updCluster.Annotations[common.NavarkosClusterStateKey] = common.NavarkosClusterStatePending
				err := dc.cController.UpdateCluster(updCluster)
				if err != nil {
					return statusError, err
				}
				glog.Infof("checkClusters updated cluster %s to state %v", pCluster.Name, common.NavarkosClusterStatePending)
				status = statusCapacityRequested

			} else {
				glog.Errorf("checkClusters Can not get latest cluster %s , %v ", pCluster.Name, err)
				return statusError, err
			}
		}
	}
	for _, uurcluster := range unselectedUnReadyClusters {
		provisioningstate := uurcluster.Annotations[common.NavarkosClusterStateKey]
		glog.V(4).Infof("checkClusters cluster from unselectedUnReadyClusters clusterstate: %s %v",
			provisioningstate, uurcluster.Name)
	}
	return status, nil
}

func getSecondsSincLastTransitionTime(cluster *v1beta1.Cluster) int {
	for _, condition := range cluster.Status.Conditions {
		if condition.Status == apiv1.ConditionTrue {
			timestamp := condition.LastTransitionTime.Unix()
			localTime := time.Now().Unix()
			return int(localTime - timestamp)
		}
	}
	return 0
}

func getPriorityClusters(notReadyClusters []*v1beta1.Cluster) ([]*v1beta1.Cluster, error) {

	if len(notReadyClusters) == 0 {
		return nil, fmt.Errorf("getPriorityClusters No Unready clusters available")
	}

	clMap := make(map[int][]*v1beta1.Cluster)
	keys := make([]int, 0)
	for _, nrCluster := range notReadyClusters {
		var priority int
		if priorityStr, ok := nrCluster.Annotations[common.NavarkosClusterPriorityKey]; ok {
			priority, _ = strconv.Atoi(priorityStr)
		} else {
			priority = common.NavarkosClusterDefaultPriority
		}
		if _, ok := clMap[priority]; !ok {
			clMap[priority] = make([]*v1beta1.Cluster, 1)
			clMap[priority][0] = nrCluster
			keys = append(keys, priority)
		} else {
			clMap[priority] = append(clMap[priority], nrCluster)
		}
	}

	sort.Ints(keys)

	glog.V(4).Infof("getPriorityClusters best left priority -  %d", keys[0])

	return clMap[keys[0]], nil

}

//GetAnnotationIntegerValue returns the value from annotation as integer
func GetAnnotationIntegerValue(obj *metav1.ObjectMeta, annotationName string, defaultValue int) int {
	annotationOriginalValue, ok := obj.Annotations[annotationName]
	if ok && annotationOriginalValue != "" {
		annotationConvertedValue, err := strconv.Atoi(annotationOriginalValue)
		if err == nil {
			return annotationConvertedValue
		}
	}
	return defaultValue
}
