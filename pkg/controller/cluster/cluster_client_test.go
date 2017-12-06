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
	"fmt"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	federation_v1beta1 "k8s.io/kubernetes/federation/apis/federation/v1beta1"
	kubeletapis "k8s.io/kubernetes/pkg/kubelet/apis"
	"net/http"
	"net/http/httptest"
	"testing"
)

// init a fake http handler, simulate a cluster apiserver, response the "/healthz"
// when "canBeGotten" is false, means that user can not get response from apiserver
func createHttptestFakeHandlerForHealthzCluster(canBeGotten bool) *http.HandlerFunc {
	fakeHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.Method {
		case "GET":
			if canBeGotten {
				fmt.Fprint(w, "ok")
			} else {
				w.WriteHeader(http.StatusNotFound)
			}
		default:
			fmt.Fprint(w, "")
		}
	})
	return &fakeHandler
}

func TestGetClusterPods(t *testing.T) {
	node := &v1.Node{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{
			UID:  uuid.NewUUID(),
			Name: "FakeNode",
		},
	}

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

	labels := map[string]string{
		kubeletapis.LabelZoneFailureDomain: "FakeZone",
		kubeletapis.LabelZoneRegion:        "FakeRegion",
	}
	node.Labels = labels

	//res := api.ResourceList{}
	//res[api.ResourcePods] = resource.MustParse("10")
	//node.Status.Capacity = res

	nodeList := &v1.NodeList{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1"},
		ListMeta: metav1.ListMeta{
			SelfLink: "foobar",
		},
		Items: []v1.Node{*node},
	}

	if len(nodeList.Items[0].Labels) != 2 {
		t.Errorf("Missing labels in nodeList")
	}
	testClusterServer := httptest.NewServer(createHttptestFakeHandlerForCluster(nodeList, true))
	defer testClusterServer.Close()

	clusterName := "foobarCluster"
	federationCluster := *newCluster(clusterName, testClusterServer.URL)
	currentTime = metav1.Now()
	newClusterReadyCondition := federation_v1beta1.ClusterCondition{
		Type:               federation_v1beta1.ClusterReady,
		Status:             v1.ConditionTrue,
		Reason:             "ClusterReady",
		Message:            "/healthz responded with ok",
		LastProbeTime:      currentTime,
		LastTransitionTime: currentTime,
	}
	federationCluster.Status.Conditions = append(federationCluster.Status.Conditions, newClusterReadyCondition)

	federationClusterList := newClusterList(&federationCluster)

	testFederationServer := httptest.NewServer(createHttptestFakeHandlerForFederation(federationClusterList, true))
	defer testFederationServer.Close()

	clusterClient, err := NewClusterClientSet(&federationCluster)
	if err != nil {
		t.Error(err)
	}

	allocatablePods, capacityPods, err := clusterClient.GetClusterPods()

	if err == nil {
		fmt.Printf("cluster allocatablePods are %v and capacityPods are %v \n", allocatablePods, capacityPods)
	} else {
		t.Error(err)
	}
}

func TestGetClusterZones(t *testing.T) {

	node := &v1.Node{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{
			UID:  uuid.NewUUID(),
			Name: "foozone",
		},
	}

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

	labels := map[string]string{
		kubeletapis.LabelZoneFailureDomain: "FakeZone",
		kubeletapis.LabelZoneRegion:        "FakeRegion",
	}
	node.Labels = labels

	nodeList := &v1.NodeList{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1"},
		ListMeta: metav1.ListMeta{
			SelfLink: "foobar",
		},
		Items: []v1.Node{*node},
	}

	if len(nodeList.Items[0].Labels) != 2 {
		t.Errorf("Missing labels in nodeList")
	}

	testClusterServer := httptest.NewServer(createHttptestFakeHandlerForCluster(nodeList, true))
	defer testClusterServer.Close()

	federationCluster := *newCluster(clusterName, testClusterServer.URL)
	currentTime = metav1.Now()
	newClusterReadyCondition := federation_v1beta1.ClusterCondition{
		Type:               federation_v1beta1.ClusterReady,
		Status:             v1.ConditionTrue,
		Reason:             "ClusterReady",
		Message:            "/healthz responded with ok",
		LastProbeTime:      currentTime,
		LastTransitionTime: currentTime,
	}
	federationCluster.Status.Conditions = append(federationCluster.Status.Conditions, newClusterReadyCondition)

	federationClusterList := newClusterList(&federationCluster)

	testFederationServer := httptest.NewServer(createHttptestFakeHandlerForFederation(federationClusterList, true))
	defer testFederationServer.Close()

	clusterClient, err := NewClusterClientSet(&federationCluster)
	if err != nil {
		t.Error(err)
	}

	zones, region, err := clusterClient.GetClusterZones()
	if err == nil {
		fmt.Printf("cluster zones are %v , region is %v \n", zones, region)
	} else {
		t.Error(err)
	}

}

func TestGetClusterHealthOkStatus(t *testing.T) {

	testClusterServer := httptest.NewServer(createHttptestFakeHandlerForHealthzCluster(true))
	defer testClusterServer.Close()

	federationCluster := *newCluster("foobarCluster", testClusterServer.URL)

	federationClusterList := newClusterList(&federationCluster)

	testFederationServer := httptest.NewServer(createHttptestFakeHandlerForFederation(federationClusterList, true))
	defer testFederationServer.Close()

	clusterClient, err := NewClusterClientSet(&federationCluster)

	if err != nil {
		t.Error(err)
	}

	clusterStatus := clusterClient.GetClusterHealthStatus()

	if len(clusterStatus.Conditions) != 1 {
		t.Errorf("Expected 1 condition, got %v", len(clusterStatus.Conditions))
	}

	if clusterStatus.Conditions[0].Type != federation_v1beta1.ClusterReady {
		t.Errorf("Expected condition %v, got condition %v", federation_v1beta1.ClusterReady, clusterStatus.Conditions[0].Type)
	}

	if clusterStatus.Conditions[0].Status != v1.ConditionTrue {
		t.Errorf("Expected condition %v, got condition %v", v1.ConditionTrue, clusterStatus.Conditions[0].Status)
	}
}
func TestGetClusterHealthOfflineStatus(t *testing.T) {

	testClusterServer := httptest.NewServer(createHttptestFakeHandlerForHealthzCluster(false))
	defer testClusterServer.Close()

	federationCluster := *newCluster("foobarCluster", testClusterServer.URL)

	federationClusterList := newClusterList(&federationCluster)

	testFederationServer := httptest.NewServer(createHttptestFakeHandlerForFederation(federationClusterList, true))
	defer testFederationServer.Close()

	clusterClient, err := NewClusterClientSet(&federationCluster)

	if err != nil {
		t.Error(err)
	}

	clusterStatus := clusterClient.GetClusterHealthStatus()

	if len(clusterStatus.Conditions) != 1 {
		t.Errorf("Expected 1 condition, got %v", len(clusterStatus.Conditions))
	}

	if clusterStatus.Conditions[0].Type != federation_v1beta1.ClusterOffline {
		t.Errorf("Expected condition %v, got condition %v", federation_v1beta1.ClusterReady, clusterStatus.Conditions[0].Type)
	}

	if clusterStatus.Conditions[0].Status != v1.ConditionTrue {
		t.Errorf("Expected condition %v, got condition %v", v1.ConditionTrue, clusterStatus.Conditions[0].Status)
	}
}
