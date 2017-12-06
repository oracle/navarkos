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
	"encoding/json"
	"fmt"
	"github.com/kubernetes-incubator/navarkos/pkg/common"
	extensionsv1 "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	fedapi "k8s.io/kubernetes/federation/apis/federation"
	"k8s.io/kubernetes/federation/apis/federation/v1beta1"
	fedtypes "k8s.io/kubernetes/federation/pkg/federatedtypes"
	"k8s.io/kubernetes/pkg/api/testapi"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetSchedule(t *testing.T) {

	dpmt1 := newDeployment("GreenDeployment", 20)
	dpmt2 := newDeployment("RedDeployment", 10)
	dpmts := []*extensionsv1.Deployment{dpmt1, dpmt2}

	testClusterServer := httptest.NewServer(createHttptestFakeHandlerForCluster(true))
	defer testClusterServer.Close()
	cluster1 := newCluster("GreenCluster", testClusterServer.URL)
	annotations1 := map[string]string{
		common.NavarkosClusterCapacityPodsKey:       "10",
		common.NavarkosClusterCapacitySystemPodsKey: "5",
	}
	cluster1.Annotations = annotations1

	cluster2 := newCluster("RedCluster", testClusterServer.URL)
	annotations2 := map[string]string{
		common.NavarkosClusterCapacityPodsKey:       "50",
		common.NavarkosClusterCapacitySystemPodsKey: "5",
	}
	cluster2.Annotations = annotations2

	cluster3 := newCluster("RedGreenCluster", testClusterServer.URL)
	annotations3 := map[string]string{
		common.NavarkosClusterPriorityKey: "2",
	}
	cluster3.Annotations = annotations3

	clusters := []*v1beta1.Cluster{cluster1, cluster2, cluster3}
	for _, cluster := range clusters {
		fmt.Printf("%v \n", cluster.Annotations)
	}

	allocation, error := GetSchedule(dpmts, clusters)
	var pref fedapi.ReplicaAllocationPreferences
	if error == nil {
		for _, deployment := range allocation {
			fmt.Printf("%v \n", deployment.Annotations[fedtypes.FedDeploymentPreferencesAnnotation])
			json.Unmarshal([]byte(deployment.Annotations[fedtypes.FedDeploymentPreferencesAnnotation]), &pref)
			if deployment.Name == dpmt1.Name {
				if pref.Clusters[cluster1.Name].MinReplicas != 3 {
					t.Error("Replica count for ", deployment.Name, " on cluster ", cluster1.Name, " is ", pref.Clusters[cluster1.Name].MinReplicas, " instead of 3")
				}
				if pref.Clusters[cluster2.Name].MinReplicas != 17 {
					t.Error("Replica count for ", deployment.Name, " on cluster ", cluster2.Name, " is ", pref.Clusters[cluster2.Name].MinReplicas, " instead of 17")
				}
			} else {
				if pref.Clusters[cluster2.Name].MinReplicas != 10 {
					t.Error("Replica count for ", deployment.Name, " on cluster ", cluster2.Name, " is ", pref.Clusters[cluster2.Name].MinReplicas, " instead of 10")
				}
			}
		}
	} else {
		t.Errorf("GetSchedule resulted in error %v", error)
	}
}

// init a fake http handler, simulate a cluster apiserver, response the "/healthz"
// when "canBeGotten" is false, means that user can not get response from apiserver
func createHttptestFakeHandlerForCluster(canBeGotten bool) *http.HandlerFunc {
	fakeHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case "GET":
			if canBeGotten {
				fmt.Fprintln(w, "ok")
			} else {
				w.WriteHeader(http.StatusNotFound)
			}
		default:
			fmt.Fprintln(w, "")
		}
	})
	return &fakeHandler
}

func newCluster(clusterName string, serverUrl string) *v1beta1.Cluster {
	cluster := v1beta1.Cluster{
		TypeMeta: metav1.TypeMeta{APIVersion: testapi.Federation.GroupVersion().String()},
		ObjectMeta: metav1.ObjectMeta{
			UID:  uuid.NewUUID(),
			Name: clusterName,
		},
		Spec: v1beta1.ClusterSpec{
			ServerAddressByClientCIDRs: []v1beta1.ServerAddressByClientCIDR{
				{
					ClientCIDR:    "0.0.0.0/0",
					ServerAddress: serverUrl,
				},
			},
		},
	}
	return &cluster
}

func newDeployment(name string, replica int) *extensionsv1.Deployment {
	replicas := int32(replica)
	return &extensionsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: metav1.NamespaceDefault,
			SelfLink:  "/api/v1/namespaces/default/deployments/name123",
		},
		Spec: extensionsv1.DeploymentSpec{
			Replicas: &replicas,
		},
	}
}
