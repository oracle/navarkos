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

package cluster

import (
	"encoding/json"
	"fmt"
	"github.com/kubernetes-incubator/navarkos/pkg/common"
	"io/ioutil"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	federationv1beta1 "k8s.io/kubernetes/federation/apis/federation/v1beta1"
	federationclientset "k8s.io/kubernetes/federation/client/clientset_generated/federation_clientset"
	"k8s.io/kubernetes/pkg/api/testapi"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newNode() *v1.Node {
	node := v1.Node{}

	currentTime := metav1.Now()
	newNodeReadyCondition := v1.NodeCondition{
		Type:               v1.NodeReady,
		Status:             v1.ConditionTrue,
		Reason:             "NodeReady",
		Message:            "/healthz responded with ok",
		LastHeartbeatTime:  currentTime,
		LastTransitionTime: currentTime,
	}

	node.Status.Conditions = append(node.Status.Conditions, newNodeReadyCondition)

	return &node
}

func newCluster(clusterName string, serverUrl string) *federationv1beta1.Cluster {
	cluster := federationv1beta1.Cluster{
		TypeMeta: metav1.TypeMeta{APIVersion: testapi.Federation.GroupVersion().String()},
		ObjectMeta: metav1.ObjectMeta{
			UID:  uuid.NewUUID(),
			Name: clusterName,
		},
		Spec: federationv1beta1.ClusterSpec{
			ServerAddressByClientCIDRs: []federationv1beta1.ServerAddressByClientCIDR{
				{
					ClientCIDR:    "0.0.0.0/0",
					ServerAddress: serverUrl,
				},
			},
		},
	}
	return &cluster
}

func newNodeList(node *v1.Node) *v1.NodeList {
	nodeList := v1.NodeList{
		Items: []v1.Node{}}

	nodeList.Items = append(nodeList.Items, *node)

	return &nodeList
}

func newClusterList(cluster *federationv1beta1.Cluster) *federationv1beta1.ClusterList {
	clusterList := federationv1beta1.ClusterList{
		TypeMeta: metav1.TypeMeta{APIVersion: testapi.Federation.GroupVersion().String()},
		ListMeta: metav1.ListMeta{
			SelfLink: "foobar",
		},
		Items: []federationv1beta1.Cluster{},
	}
	clusterList.Items = append(clusterList.Items, *cluster)
	return &clusterList
}

// init a fake http handler, simulate a federation apiserver, response the "DELETE" "PUT" "GET" "UPDATE"
// when "canBeGotten" is false, means that user can not get the cluster cluster from apiserver
func createHttptestFakeHandlerForFederation(clusterList *federationv1beta1.ClusterList, canBeGotten bool) *http.HandlerFunc {
	fakeHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		w.Header().Set("Content-Type", "application/json")

		switch r.Method {
		case "PUT":
			body, _ := ioutil.ReadAll(r.Body)
			var newCluster federationv1beta1.Cluster
			json.Unmarshal(body, &newCluster)

			rv, _ := json.Marshal(newCluster)
			fmt.Fprintf(w, string(rv))
		case "GET":
			if canBeGotten {
				if strings.HasSuffix(r.RequestURI, "/clusters/"+clusterName) {
					clusterString, _ := json.Marshal(clusterList.Items[0])
					fmt.Fprintln(w, string(clusterString))
				} else {
					clusterListString, _ := json.Marshal(*clusterList)

					fmt.Fprintln(w, string(clusterListString))
				}
			} else {
				fmt.Fprintln(w, "")
			}
		default:
			fmt.Fprintln(w, "")
		}
	})
	return &fakeHandler
}

// init a fake http handler, simulate a cluster apiserver, response the "/healthz"
// when "canBeGotten" is false, means that user can not get response from apiserver
func createHttptestFakeHandlerForCluster(nodeList *v1.NodeList, canBeGotten bool) *http.HandlerFunc {
	fakeHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		nodeListString, _ := json.Marshal(*nodeList)

		switch r.Method {
		case "GET":
			if canBeGotten {
				fmt.Fprintln(w, string(nodeListString))
			} else {
				w.WriteHeader(http.StatusNotFound)
			}
		default:
			fmt.Fprintln(w, "")
		}
	})
	return &fakeHandler
}

const clusterName = "foobarCluster"

func TestUpdateClusterStatusOK(t *testing.T) {

	clusterController, testServer1, testServer2, err := createTestClusterController()

	defer testServer1.Close()
	defer testServer2.Close()

	if err != nil {
		t.Errorf("Failed to create ClusterController: %v", err)
	}

	err = clusterController.UpdateClusterStatus()
	if err != nil {
		t.Errorf("Failed to Update Cluster Status: %v", err)
	}
}

func createTestClusterController() (*ClusterController, *httptest.Server, *httptest.Server, error) {
	node := newNode()
	nodeList := newNodeList(node)
	testClusterServer := httptest.NewServer(createHttptestFakeHandlerForCluster(nodeList, true))

	federationCluster := newCluster(clusterName, testClusterServer.URL)
	currentTime := metav1.Now()
	newClusterReadyCondition := federationv1beta1.ClusterCondition{
		Type:               federationv1beta1.ClusterReady,
		Status:             v1.ConditionTrue,
		Reason:             "ClusterReady",
		Message:            "/healthz responded with ok",
		LastProbeTime:      currentTime,
		LastTransitionTime: currentTime,
	}
	federationCluster.Status.Conditions = append(federationCluster.Status.Conditions, newClusterReadyCondition)
	federationClusterList := newClusterList(federationCluster)
	testFederationServer := httptest.NewServer(createHttptestFakeHandlerForFederation(federationClusterList, true))

	restClientCfg, err := clientcmd.BuildConfigFromFlags(testFederationServer.URL, "")
	if err != nil {
		return nil, testClusterServer, testFederationServer, err
	}
	federationClientSet := federationclientset.NewForConfigOrDie(restclient.AddUserAgent(restClientCfg, "Navarkos-Cluster-Controller"))
	manager := newClusterController(federationClientSet, 5)
	return manager, testClusterServer, testFederationServer, err
}

func TestGetReadyClusters(t *testing.T) {
	clusterController, testServer1, testServer2, err := createTestClusterController()

	defer testServer1.Close()
	defer testServer2.Close()

	stopChan := make(chan struct{})
	if err != nil {
		t.Errorf("Failed to create ClusterController: %v", err)
	}

	clusterController.Run(stopChan)

	defer close(stopChan)

	for !clusterController.clusterController.HasSynced() {
		time.Sleep(5)
	}

	clusters, err := clusterController.GetReadyClusters()

	if err != nil {
		t.Errorf("Error getting ready clusters: %v", err)
	}

	if len(clusters) != 1 {
		t.Errorf("Expected 1 cluster, got %v", len(clusters))
	}
}

func TestGetUnreadyClusters(t *testing.T) {
	clusterController, testServer1, testServer2, err := createTestClusterController()

	defer testServer1.Close()
	defer testServer2.Close()

	stopChan := make(chan struct{})
	if err != nil {
		t.Errorf("Failed to create ClusterController: %v", err)
	}

	clusterController.Run(stopChan)

	defer close(stopChan)

	for !clusterController.clusterController.HasSynced() {
		time.Sleep(5)
	}

	clusters, err := clusterController.GetUnreadyClusters()

	if err != nil {
		t.Errorf("Error getting ready clusters: %v", err)
	}

	if len(clusters) != 0 {
		t.Errorf("Expected 0 clusters, got %v", len(clusters))
	}
}

func TestIsClusterScaling(t *testing.T) {
	clusterController, testServer1, testServer2, err := createTestClusterController()

	defer testServer1.Close()
	defer testServer2.Close()

	stopChan := make(chan struct{})
	if err != nil {
		t.Errorf("Failed to create ClusterController: %v", err)
	}

	clusterController.Run(stopChan)

	defer close(stopChan)

	for !clusterController.clusterController.HasSynced() {
		time.Sleep(5)
	}

	cluster, err := clusterController.GetCluster(clusterName)

	if err != nil {
		fmt.Printf("Could not get cluster: %v", err)
	}

	isScaling := clusterController.IsClusterScaling(cluster)

	if isScaling {
		t.Errorf("Cluster does not have annotation %v set, so should not be scaling", common.NavarkosClusterStateKey)
	}

	// all the following annotation values should result in IsClusterScaling returning true:
	vs := []string{common.NavarkosClusterStatePendingScaleUp,
		common.NavarkosClusterStatePendingScaleDown,
		common.NavarkosClusterStateScalingDown,
		common.NavarkosClusterStateScalingUp}

	cluster.Annotations = make(map[string]string)

	for _, v := range vs {
		cluster.Annotations[common.NavarkosClusterStateKey] = v

		if !clusterController.IsClusterScaling(cluster) {
			t.Errorf("Cluster has annotation %v set to %v, should be Scaling", common.NavarkosClusterStateKey, common.NavarkosClusterStateKey)
		}
	}

	cluster.Annotations[common.NavarkosClusterStateKey] = "junk"

	if clusterController.IsClusterScaling(cluster) {
		t.Errorf("Cluster has annotation %v set to bad value, should not be Scaling", common.NavarkosClusterStateKey)
	}
}

// No validation here, just testing that execution completes in does not panic
func TestStartClusterController(t *testing.T) {
	stopChan := make(chan struct{})
	restClientCfg := &restclient.Config{}
	StartClusterController(restClientCfg, stopChan, time.Second*10)
	close(stopChan)
}

func TestMarkClusterForScalingIfPodsHitThresholds(t *testing.T) {
	clusterController, testServer1, testServer2, err := createTestClusterController()

	defer testServer1.Close()
	defer testServer2.Close()

	stopChan := make(chan struct{})
	if err != nil {
		t.Errorf("Failed to create ClusterController: %v", err)
	}

	clusterController.Run(stopChan)

	defer close(stopChan)

	for !clusterController.clusterController.HasSynced() {
		time.Sleep(5)
	}

	cluster, err := clusterController.GetCluster(clusterName)

	if err != nil {
		fmt.Printf("Could not get cluster: %v", err)
	}
	annotations := map[string]string{
		common.NavarkosClusterCapacityPodsKey:       "15",
		common.NavarkosClusterCapacitySystemPodsKey: "5",
		common.NavarkosClusterCapacityUsedPodsKey:   "3",
		common.NavarkosClusterAutoScaleKey:          "True",
		common.NavarkosClusterStateKey:              common.NavarkosClusterStateReady,
	}
	cluster.Annotations = annotations

	status := clusterController.markClusterForScalingIfPodsHitThresholds(cluster)

	//TODO: Improve log/check with test table for different Pod values
	if status {
		t.Logf("markClusterForScalingIfPodsHitThresholds: NavarkosClusterState for cluster is updated %v \n", cluster.Annotations[common.NavarkosClusterStateKey])
	} else {
		t.Logf("markClusterForScalingIfPodsHitThresholds: NavarkosClusterState for cluster is not updated %v \n", cluster.Annotations[common.NavarkosClusterStateKey])
	}
}

func TestShutDownClusterIfTTLExpired(t *testing.T) {
	clusterController, testServer1, testServer2, err := createTestClusterController()

	defer testServer1.Close()
	defer testServer2.Close()

	stopChan := make(chan struct{})
	if err != nil {
		t.Errorf("Failed to create ClusterController: %v", err)
	}

	clusterController.Run(stopChan)

	defer close(stopChan)

	for !clusterController.clusterController.HasSynced() {
		time.Sleep(5)
	}

	cluster, err := clusterController.GetCluster(clusterName)

	if err != nil {
		fmt.Printf("Could not get cluster: %v", err)
	}

	cluster.Annotations = make(map[string]string)
	cluster.Annotations[common.NavarkosClusterShutdownStartTimeKey] = time.Now().UTC().Format(time.UnixDate)
	cluster.Annotations[common.NavarkosClusterTimeToLiveBeforeShutdownKey] = "5"
	cluster.Annotations[common.NavarkosClusterStateKey] = common.NavarkosClusterStateReady
	time.Sleep(time.Second * 5)
	status := clusterController.shutDownClusterIfTTLExpired(cluster)
	if status {
		t.Logf("TestShutDownClusterIfTTLExpired: NavarkosClusterState for cluster is updated %v \n", cluster.Annotations[common.NavarkosClusterStateKey])
	} else {
		t.Errorf("TestShutDownClusterIfTTLExpired: NavarkosClusterState for cluster is not updated %v \n", cluster.Annotations[common.NavarkosClusterStateKey])
	}
}

func TestUpdateCluster(t *testing.T) {
	clusterController, testServer1, testServer2, err := createTestClusterController()

	defer testServer1.Close()
	defer testServer2.Close()

	if err != nil {
		t.Errorf("Failed to create ClusterController: %v", err)
	}

	cluster, _ := clusterController.GetCluster(clusterName)

	cluster2 := newCluster("foobar2", "")
	cluster2.Annotations = make(map[string]string)
	cluster2.Annotations[common.NavarkosClusterStateKey] = common.NavarkosClusterStateReady
	clusterController.updateCluster(cluster, cluster2)

	_, ok := clusterController.clusterKubeClientMap["foobar2"]

	if !ok {
		t.Errorf("kubeClientMap was not updated ")
	}

	cluster2.Annotations[common.NavarkosClusterStateKey] = common.NavarkosClusterStateOffline

	clusterController.updateCluster(cluster, cluster2)

	_, found := clusterController.clusterKubeClientMap["foobar2"]

	if found {
		t.Errorf("kubeClientMap was not updated - cluster was offline and should have been deleted")
	}

}

func TestMarkClusterToShutDownIfNoPods(t *testing.T) {
	clusterController, testServer1, testServer2, err := createTestClusterController()

	defer testServer1.Close()
	defer testServer2.Close()

	stopChan := make(chan struct{})
	if err != nil {
		t.Errorf("Failed to create ClusterController: %v", err)
	}

	clusterController.Run(stopChan)

	defer close(stopChan)

	for !clusterController.clusterController.HasSynced() {
		time.Sleep(5)
	}

	cluster, err := clusterController.GetCluster(clusterName)

	if err != nil {
		fmt.Printf("Could not get cluster: %v", err)
	}
	annotations := map[string]string{
		common.NavarkosClusterCapacityPodsKey:       "15",
		common.NavarkosClusterCapacitySystemPodsKey: "5",
		common.NavarkosClusterCapacityUsedPodsKey:   "5",
		common.NavarkosClusterAutoScaleKey:          "True",
		common.NavarkosClusterStateKey:              common.NavarkosClusterStateReady,
	}
	cluster.Annotations = annotations

	status := clusterController.markClusterToShutDownIfNoPods(cluster)

	if status {
		t.Logf("markClusterToShutDownIfNoPods: has marked Cluster %s for shrinking starting on %s \n", cluster.Annotations[common.NavarkosClusterShutdownStartTimeKey], cluster.Name)
	} else {
		t.Errorf("markClusterToShutDownIfNoPods: has not marked Cluster %s for shrinking   \n", cluster.Name)
	}

	delete(cluster.Annotations, common.NavarkosClusterShutdownStartTimeKey)
	cluster.Annotations[common.NavarkosClusterCapacityUsedPodsKey] = "7"
	status = clusterController.markClusterToShutDownIfNoPods(cluster)

	if status {
		t.Errorf("markClusterToShutDownIfNoPods: has marked Cluster %s for shrinking starting on %s \n", cluster.Annotations[common.NavarkosClusterShutdownStartTimeKey], cluster.Name)
	} else {
		t.Logf("markClusterToShutDownIfNoPods: has not marked Cluster %s for shrinking   \n", cluster.Name)
	}
}

func TestGetClusterDeployment(t *testing.T) {
	clusterController, testServer1, testServer2, err := createTestClusterController()

	defer testServer1.Close()
	defer testServer2.Close()

	stopChan := make(chan struct{})
	if err != nil {
		t.Errorf("Failed to create ClusterController: %v", err)
	}

	clusterController.Run(stopChan)

	defer close(stopChan)

	for !clusterController.clusterController.HasSynced() {
		time.Sleep(5)
	}

	cluster, err := clusterController.GetCluster(clusterName)

	if err != nil {
		fmt.Printf("Could not get cluster: %v", err)
	}

	d1, err := clusterController.GetClusterDeployment(cluster, "TestNamespace", "TestDeployment")
	if err == nil {
		t.Logf("Deployment is %v \n", d1)
	} else {
		t.Errorf("GetClusterDeployment failed with %v \n", err)
	}

	//TODO: Negative test for non ready cluster
	/*
		currentTime := metav1.Now()
		newClusterNotReadyCondition := federationv1beta1.ClusterCondition{
			Type:               federationv1beta1.ClusterReady,
			Status:             v1.ConditionFalse,
			Reason:             "ClusterNotReady",
			Message:            "/healthz responded without ok",
			LastProbeTime:      currentTime,
			LastTransitionTime: currentTime,
		}
		cluster.Status.Conditions := newClusterNotReadyCondition
		d1, err = clusterController.GetClusterDeployment(cluster, "TestNamespace", "TestDeployment")
		if(err == nil) {
			t.Errorf("Deployment is %v \n", d1)
		} else {
			t.Logf("GetClusterDeployment got expected error %v \n", err)
		}
	*/
}

func TestDelFromClusterSet(t *testing.T) {
	clusterController, testServer1, testServer2, err := createTestClusterController()

	defer testServer1.Close()
	defer testServer2.Close()

	stopChan := make(chan struct{})
	if err != nil {
		t.Errorf("Failed to create ClusterController: %v", err)
	}

	clusterController.Run(stopChan)

	defer close(stopChan)

	for !clusterController.clusterController.HasSynced() {
		time.Sleep(5)
	}

	cluster, err := clusterController.GetCluster(clusterName)

	if err != nil {
		fmt.Printf("Could not get cluster: %v", err)
	}

	_, ok := clusterController.clusterKubeClientMap[cluster.Name]
	if !ok {
		t.Errorf("kubeClientMap was not updated ")
	}

	clusterController.delFromClusterSet(cluster)

	_, ok = clusterController.clusterKubeClientMap[cluster.Name]
	if ok {
		t.Errorf("kubeClientMap was not updated ")
	}
}

func TestIsStatsChanged(t *testing.T) {
	clusterController, testServer1, testServer2, err := createTestClusterController()

	defer testServer1.Close()
	defer testServer2.Close()

	if err != nil {
		t.Errorf("Failed to create ClusterController: %v", err)
	}

	cluster, _ := clusterController.GetCluster(clusterName)
	annotations := map[string]string{
		common.NavarkosClusterCapacityPodsKey:       "15",
		common.NavarkosClusterCapacitySystemPodsKey: "5",
		common.NavarkosClusterCapacityUsedPodsKey:   "5",
	}
	cluster.Annotations = annotations

	stats := map[string]int{
		common.NavarkosClusterCapacityAllocatablePodsKey: 5,
	}

	status := clusterController.isStatsChanged(stats, cluster)
	if !status {
		t.Errorf("isStatsChanged: did not update Cluster %s correctly   \n", cluster.Name)
	}

}
