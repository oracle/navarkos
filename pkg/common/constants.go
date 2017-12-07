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

package common

/*
Common constant used to define annotation keys and default values
as well as interface with other projects
*/

const (
	NavarkosClusterStateKey                            = "n6s.io/cluster.lifecycle.state" //communicates the cluster life cycle state
	NavarkosClusterStateOffline                        = "offline"
	NavarkosClusterStatePending                        = "pending-provision"
	NavarkosClusterStateReady                          = "ready"
	NavarkosClusterStatePendingShutdown                = "pending-shutdown"
	NavarkosClusterStatePendingScaleUp                 = "pending-up"
	NavarkosClusterStatePendingScaleDown               = "pending-down"
	NavarkosClusterStateProvisioning                   = "provisioning"
	NavarkosClusterStateJoining                        = "joining"
	NavarkosClusterStateShuttingDown                   = "shutting-down"
	NavarkosClusterStateScalingUp                      = "scaling-up"
	NavarkosClusterStateScalingDown                    = "scaling-down"
	NavarkosClusterStateFailedProvision                = "failed-provision"
	NavarkosClusterStateFailedScaleUp                  = "failed-up"
	NavarkosClusterStateFailedScaleDown                = "failed-down"
	NavarkosClusterCapacityAllocatablePodsKey          = "n6s.io/cluster.capacity.allocatable-pods"
	NavarkosClusterCapacityPodsKey                     = "n6s.io/cluster.capacity.total-capacity-pods"
	NavarkosClusterCapacityUsedPodsKey                 = "n6s.io/cluster.capacity.used-pods"
	NavarkosClusterCapacitySystemPodsKey               = "n6s.io/cluster.capacity.used-system-pods"
	NavarkosClusterPriorityKey                         = "n6s.io/cluster.priority"
	NavarkosClusterDefaultPriority                 int = 1
	NavarkosClusterShutdownStartTimeKey                = "n6s.io/cluster.idle-start-timestamp"
	NavarkosClusterTimeToLiveBeforeShutdownKey         = "n6s.io/cluster.idle-time-to-live" // if set to a value of less than or equal 0, it will disable this
	NavarkosDefaultClusterTimeToLiveBeforeShutdown     = 1200                               // 20 minutes - default value in seconds for cluster to be empty to trigger shutdown
	NavarkosClusterAutoScaleKey                        = "n6s.io/cluster.autoscale.enabled"
	NavarkosClusterScaleUpCapacityThresholdKey         = "n6s.io/cluster.autoscale.scale-up-threshold"
	NavarkosDefaultClusterScaleUpCapacityThreshold     = 80 // default value in Percentage to trigger cluster scale up
	NavarkosClusterScaleDownCapacityThresholdKey       = "n6s.io/cluster.autoscale.scale-down-threshold"
	NavarkosClusterScaleDownCapacityThreshold          = 10 // default value in Percentage to trigger cluster scale down
	NavarkosClusterNodeCountKey                        = "n6s.io/cluster.node-count"
)
