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
	"github.com/golang/glog"
	"github.com/kubernetes-incubator/navarkos/pkg/common"
	extensionsv1 "k8s.io/api/extensions/v1beta1"
	fedapi "k8s.io/kubernetes/federation/apis/federation"
	v1beta1 "k8s.io/kubernetes/federation/apis/federation/v1beta1"
	fedtypes "k8s.io/kubernetes/federation/pkg/federatedtypes"
	"k8s.io/kubernetes/federation/pkg/federation-controller/util/clusterselector"
)

//ClusterData holds extracted cluster capacity data
type ClusterData struct {
	Name          string
	TotalCapacity int
	UsedCapacity  int
	Priority      int
	Labels        map[string]string
}

//DeploymentData holds extracted deployment distribution data
type DeploymentData struct {
	Name                string
	Replicas            int
	UnscheduledReplicas int
	Annotations         map[string]string
}

//GetSchedule figures out how given deployment should be distributed across ready cluster
func GetSchedule(deployments []*extensionsv1.Deployment, readyClusters []*v1beta1.Cluster) ([]*extensionsv1.Deployment, error) {

	var dpmts []*DeploymentData
	var clusters []*ClusterData
	var minFreePodsOnCluster int

	for _, dpmt := range deployments {
		dpmtData := &DeploymentData{
			Name:                dpmt.Name,
			Replicas:            int(*dpmt.Spec.Replicas),
			UnscheduledReplicas: 0,
			Annotations:         dpmt.Annotations,
		}

		if dpmt.Spec.Strategy.Type == extensionsv1.RollingUpdateDeploymentStrategyType {
			minFreePodsOnCluster += int(dpmt.Spec.Strategy.RollingUpdate.MaxSurge.IntVal)
		} else if dpmt.Spec.Strategy.Type == extensionsv1.RecreateDeploymentStrategyType {
			minFreePodsOnCluster += int(*dpmt.Spec.Replicas)
		} else {
			minFreePodsOnCluster++
		}

		dpmts = append(dpmts, dpmtData)
	}

	for _, cluster := range readyClusters {
		clusterData := &ClusterData{
			Name:          cluster.Name,
			TotalCapacity: (GetAnnotationIntegerValue(&cluster.ObjectMeta, common.NavarkosClusterCapacityPodsKey, 0) - GetAnnotationIntegerValue(&cluster.ObjectMeta, common.NavarkosClusterCapacitySystemPodsKey, 0)),
			Priority:      GetAnnotationIntegerValue(&cluster.ObjectMeta, common.NavarkosClusterPriorityKey, common.NavarkosClusterDefaultPriority),
			UsedCapacity:  0,
			Labels:        cluster.Labels,
		}
		clusters = append(clusters, clusterData)
	}

	var result []*extensionsv1.Deployment

	allocation := allocate(dpmts, clusters, minFreePodsOnCluster)
	fedPrefs := cast2fedPref(allocation)

	for _, deployment := range deployments {
		dpmtPrefs, ok := deployment.Annotations[fedtypes.FedDeploymentPreferencesAnnotation]
		if !ok || dpmtPrefs != fedPrefs[deployment.Name] {
			if deployment.Annotations == nil {
				deployment.Annotations = make(map[string]string)
			}
			deployment.Annotations[fedtypes.FedDeploymentPreferencesAnnotation] = fedPrefs[deployment.Name]
			result = append(result, deployment)
		}
	}

	return result, nil
}

func allocate(deployments []*DeploymentData, clusters []*ClusterData, minFreePods int) map[string]map[string]int {

	allocation := make(map[string]map[string]int)
	//init the map
	for _, dpmt := range deployments {
		dpmt.UnscheduledReplicas = dpmt.Replicas
		_, found := allocation[dpmt.Name]
		if !found {
			allocation[dpmt.Name] = make(map[string]int)
		}
	}

	maxPriority := getMaxPriority(clusters)

	for i := 1; i <= maxPriority && !isAllReplicasAllocated(deployments); i++ {
		keepGoing := true
		for keepGoing {
			keepGoing = false
			for _, dpmt := range deployments {
				if dpmt.UnscheduledReplicas > 0 {
					selectedClusters := selectClusters(dpmt.Annotations, clusters, i, minFreePods)
					if selectedClusters != nil && len(selectedClusters) > 0 {
						bestFit := put2BestFit(selectedClusters)
						_, found := allocation[dpmt.Name][bestFit.Name]
						if !found {
							allocation[dpmt.Name][bestFit.Name] = 1
						} else {
							allocation[dpmt.Name][bestFit.Name]++
						}
						dpmt.UnscheduledReplicas--
						keepGoing = true
					} else {
						glog.V(4).Infof("No seleactbale clusters left")
					}
				}
			}
		}
	}
	return allocation
}

func isAllReplicasAllocated(deployments []*DeploymentData) bool {
	for _, dpmt := range deployments {
		if dpmt.UnscheduledReplicas > 0 {
			return false
		}
	}
	return true
}

func getMaxPriority(clusters []*ClusterData) int {
	var maxPriority int = 1
	for _, cluster := range clusters {
		if cluster.Priority > maxPriority {
			maxPriority = cluster.Priority
		}
	}
	return maxPriority
}

func cast2fedPref(allocation map[string]map[string]int) map[string]string {
	result := make(map[string]string)

	for dpmtName, clusterAlloc := range allocation {
		var weightPerClusterMap = make(map[string]fedapi.ClusterPreferences)
		for cluster, alloc := range clusterAlloc {
			var maxReplicas int64
			maxReplicas = int64(alloc)
			weightPerClusterMap[cluster] = fedapi.ClusterPreferences{MaxReplicas: &maxReplicas, MinReplicas: maxReplicas, Weight: 1}
		}
		fedPref := &fedapi.ReplicaAllocationPreferences{Rebalance: true, Clusters: weightPerClusterMap}
		marshalledPref, err := json.Marshal(fedPref)
		if err != nil {
			glog.Errorf("Error marshalling cluster preferences %v", err)
			continue
		}
		result[dpmtName] = string(marshalledPref)
	}

	return result
}

func selectClusters(annotations map[string]string, clusters []*ClusterData, priority int, minFreePods int) []*ClusterData {
	var result []*ClusterData
	for _, cluster := range clusters {
		if cluster.Priority == priority {
			send, err := clusterselector.SendToCluster(cluster.Labels, annotations)
			if err != nil {
				//TODO: handle error
				return nil
			} else if send {
				// leave some room
				if (cluster.TotalCapacity - cluster.UsedCapacity) > minFreePods {
					result = append(result, cluster)
				} else {
					glog.V(4).Infof("Cluster %s is out of scheduled capacity", cluster.Name)
				}
			}
		}
	}
	return result
}

func put2BestFit(clusters []*ClusterData) *ClusterData {
	var minPercentUsed float32 = 100
	var bestFit *ClusterData = clusters[0]
	for _, cluster := range clusters {
		var percentUsed float32 = (float32(cluster.UsedCapacity+1) / float32(cluster.TotalCapacity)) * 100.0
		if percentUsed < minPercentUsed {
			minPercentUsed = percentUsed
			bestFit = cluster
		}
	}
	bestFit.UsedCapacity++
	return bestFit
}
