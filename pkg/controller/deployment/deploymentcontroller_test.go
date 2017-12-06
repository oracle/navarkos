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
	"testing"
	//federationclientset "k8s.io/kubernetes/federation/client/clientset_generated/federation_clientset"
	fakefedclientset "k8s.io/kubernetes/federation/client/clientset_generated/federation_clientset/fake"
	//kubeclientset "k8s.io/kubernetes/pkg/client/clientset_generated/clientset"
	//fakekubeclientset "k8s.io/kubernetes/pkg/client/clientset_generated/clientset/fake"
	"encoding/json"
	"fmt"
	"github.com/kubernetes-incubator/navarkos/pkg/common"
	"github.com/kubernetes-incubator/navarkos/pkg/controller/cluster"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/apimachinery/pkg/util/wait"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	fedtypes "k8s.io/kubernetes/federation/pkg/federatedtypes"
	"k8s.io/kubernetes/pkg/api/testapi"
	"net/http"
	"net/http/httptest"
	"time"
	//extensionsv1 "k8s.io/api/extensions/v1beta1"
	//federationv1beta1 "k8s.io/kubernetes/federation/apis/federation/v1beta1"
	federationapi "k8s.io/kubernetes/federation/apis/federation/v1beta1"
	fedtest "k8s.io/kubernetes/federation/pkg/federation-controller/util/test"
	//"k8s.io/apimachinery/pkg/runtime"
	//"k8s.io/apimachinery/pkg/types"
	//"github.com/stretchr/testify/assert"
	//"k8s.io/client-go/kubernetes"
)

const (
	deployments = "deployments"
)

func TestDeploymentController(t *testing.T) {

	testClusterServer := httptest.NewServer(createHttptestFakeHandlerForCluster(true))
	defer testClusterServer.Close()
	cluster1 := getCluster("foobarCluster", testClusterServer.URL)
	annotations1 := map[string]string{
		common.NavarkosClusterCapacityPodsKey:       "10",
		common.NavarkosClusterCapacitySystemPodsKey: "5",
	}
	cluster1.Annotations = annotations1
	federationClusterList := newClusterList(cluster1)

	testFederationServer := httptest.NewServer(createHttptestFakeHandlerForFederation(federationClusterList, true))
	defer testFederationServer.Close()
	restClientCfg, err := clientcmd.BuildConfigFromFlags(testFederationServer.URL, "")
	if err != nil {
		t.Errorf("Failed to build client config \n")
	}

	stopChan := wait.NeverStop
	cc := cluster.StartClusterController(restClientCfg, stopChan, time.Second*10)
	//cc := &cluster.ClusterController{}

	restclient.AddUserAgent(restClientCfg, "navarkos-deployment-controller")
	//fakeClient := federationclientset.NewForConfigOrDie(restClientCfg)
	fakeClient := &fakefedclientset.Clientset{}
	dc := newDeploymentController(fakeClient, cc)

	// Add an update reactor on fake client to return the desired updated object.
	// This is a hack to workaround https://github.com/kubernetes/kubernetes/issues/40939.
	fedtest.AddFakeUpdateReactor(deployments, &fakeClient.Fake)
	fedtest.RegisterFakeList("clusters", &fakeClient.Fake, &federationapi.ClusterList{Items: []federationapi.Cluster{*cluster1}})
	deploymentsWatch := fedtest.RegisterFakeWatch(deployments, &fakeClient.Fake)
	/*
		clusterWatch := fedtest.RegisterFakeWatch("clusters", &fakeClient.Fake)

		cluster1Client := &fakekubeclientset.Clientset{}
		cluster1Watch := fedtest.RegisterFakeWatch(deployments, &cluster1Client.Fake)
		_ = fedtest.RegisterFakeWatch(pods, &cluster1Client.Fake)
		fedtest.RegisterFakeList(deployments, &cluster1Client.Fake, &extensionsv1.DeploymentList{Items: []extensionsv1.Deployment{}})
		cluster1CreateChan := fedtest.RegisterFakeCopyOnCreate(deployments, &cluster1Client.Fake, cluster1Watch)
		cluster1UpdateChan := fedtest.RegisterFakeCopyOnUpdate(deployments, &cluster1Client.Fake, cluster1Watch)

		clientFactory := func(cluster *federationapi.Cluster) (kubernetes.Interface, error) {
			switch cluster.Name {
			case cluster1.Name:
				return &fakekubeclientset.Clientset{}, nil
			default:
				return nil, fmt.Errorf("Unknown cluster")
			}
		}
		fedtest.ToFederatedInformerForTestOnly(dc.informer).SetClientFactory(clientFactory)
	*/

	fmt.Printf("default reviewDelay is %v \n", dc.reviewDelay)
	dc.minimizeLatency()
	fmt.Printf("minimized reviewDelay is %v \n", dc.reviewDelay)
	go dc.Run(stopChan)

	dep1 := newDeployment("GreenDeployment", 20)
	deploymentsWatch.Add(dep1)
	time.Sleep(time.Second * 3)
	fmt.Printf("%v \n", dep1.Annotations[fedtypes.FedDeploymentPreferencesAnnotation])
	/*
		deploymentsWatch.Add(dep1)
		checkDeployment := func(base *extensionsv1.Deployment, replicas int32) fedtest.CheckingFunction {
			return func(obj runtime.Object) error {
				if obj == nil {
					return fmt.Errorf("Observed object is nil")
				}
				d := obj.(*extensionsv1.Deployment)
				if err := fedtest.CompareObjectMeta(base.ObjectMeta, d.ObjectMeta); err != nil {
					return err
				}
				if replicas != *d.Spec.Replicas {
					return fmt.Errorf("Replica count is different expected:%d observed:%d", replicas, *d.Spec.Replicas)
				}
				return nil
			}
		}
		assert.NoError(t, fedtest.CheckObjectFromChan(cluster1CreateChan, checkDeployment(dep1, *dep1.Spec.Replicas)))
		err = fedtest.WaitForStoreUpdate(
			dc.informer.GetTargetStore(),
			cluster1.Name, types.NamespacedName{Namespace: dep1.Namespace, Name: dep1.Name}.String(), wait.ForeverTestTimeout)
		assert.Nil(t, err, "deployment should have appeared in the informer store")
	*/
}

// init a fake http handler, simulate a federation apiserver, response the "DELETE" "PUT" "GET" "UPDATE"
// when "canBeGotten" is false, means that user can not get the cluster cluster from apiserver
func createHttptestFakeHandlerForFederation(clusterList *federationapi.ClusterList, canBeGotten bool) *http.HandlerFunc {
	fakeHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clusterListString, _ := json.Marshal(*clusterList)
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case "PUT":
			fmt.Fprintln(w, string(clusterListString))
		case "GET":
			if canBeGotten {
				fmt.Fprintln(w, string(clusterListString))
			} else {
				fmt.Fprintln(w, "")
			}
		default:
			fmt.Fprintln(w, "")
		}
	})
	return &fakeHandler
}

func newClusterList(cluster *federationapi.Cluster) *federationapi.ClusterList {
	clusterList := federationapi.ClusterList{
		TypeMeta: metav1.TypeMeta{APIVersion: testapi.Federation.GroupVersion().String()},
		ListMeta: metav1.ListMeta{
			SelfLink: "foobar",
		},
		Items: []federationapi.Cluster{},
	}
	clusterList.Items = append(clusterList.Items, *cluster)
	return &clusterList
}

func getCluster(clusterName string, serverUrl string) *federationapi.Cluster {
	cluster := federationapi.Cluster{
		TypeMeta: metav1.TypeMeta{APIVersion: testapi.Federation.GroupVersion().String()},
		ObjectMeta: metav1.ObjectMeta{
			UID:  uuid.NewUUID(),
			Name: clusterName,
		},
		Spec: federationapi.ClusterSpec{
			ServerAddressByClientCIDRs: []federationapi.ServerAddressByClientCIDR{
				{
					ClientCIDR:    "0.0.0.0/0",
					ServerAddress: serverUrl,
				},
			},
		},
	}
	return &cluster
}

// No validation here, just testing that execution completes in does not panic
func TestStartDeploymentController(t *testing.T) {
	stopChan := make(chan struct{})
	restClientCfg := &restclient.Config{}
	cc := cluster.StartClusterController(restClientCfg, stopChan, time.Second*10)
	StartDeploymentController(cc, restClientCfg, stopChan, true)
	close(stopChan)
}

func TestGetSecondsSincLastTransitionTime(t *testing.T) {
	testClusterServer := httptest.NewServer(createHttptestFakeHandlerForCluster(true))
	defer testClusterServer.Close()
	federationCluster := getCluster("foobarCluster", testClusterServer.URL)
	currentTime := metav1.Now()
	newClusterReadyCondition := federationapi.ClusterCondition{
		Type:               federationapi.ClusterReady,
		Status:             v1.ConditionTrue,
		Reason:             "ClusterReady",
		Message:            "/healthz responded with ok",
		LastProbeTime:      currentTime,
		LastTransitionTime: currentTime,
	}
	federationCluster.Status.Conditions = append(federationCluster.Status.Conditions, newClusterReadyCondition)
	time.Sleep(time.Second * 3)
	ltt := getSecondsSincLastTransitionTime(federationCluster)

	if ltt == 0 {
		t.Errorf("SecondsSincLastTransitionTime should have been atleast 3 seconds, but its 0 seconds")
	}
}

func TestGetPriorityClusters(t *testing.T) {
	testClusterServer := httptest.NewServer(createHttptestFakeHandlerForCluster(true))
	defer testClusterServer.Close()

	cluster1 := getCluster("foobarCluster1", testClusterServer.URL)
	annotations1 := map[string]string{
		common.NavarkosClusterPriorityKey: "2",
	}
	cluster1.Annotations = annotations1

	cluster2 := getCluster("foobarCluster2", testClusterServer.URL)
	annotations2 := map[string]string{
		common.NavarkosClusterPriorityKey: "2",
	}
	cluster2.Annotations = annotations2

	cluster3 := getCluster("foobarCluster3", testClusterServer.URL)
	annotations3 := map[string]string{
		common.NavarkosClusterPriorityKey: "3",
	}
	cluster3.Annotations = annotations3

	cluster4 := getCluster("foobarCluster4", testClusterServer.URL)

	priorityClusters, err := getPriorityClusters([]*federationapi.Cluster{cluster1, cluster2, cluster3, cluster4})

	if err == nil {
		if len(priorityClusters) != 1 {
			t.Errorf("getPriorityClusters returned more than one cluster %v \n", len(priorityClusters))
		} else {
			t.Logf("getPriorityClusters returned expected cluster %v \n", priorityClusters[0].Name)
		}
	} else {
		t.Errorf("getPriorityClusters failed with %v \n", err)
	}
}
