# Examples to show how to get started with Navarkos and use it

## Prerequisites

- Following example instructions assumes that you have Navarkos deployed. See [Deploy instructions](../deploy/README.md) for more details.
- Read [Federated Deployment](https://kubernetes.io/docs/tasks/administer-federation/deployment/) and [Federated Cluster](https://kubernetes.io/docs/tasks/administer-federation/cluster/) and understand how default scheduling of demand (deployment replicas) and supply (federated clusters) works.
- Read [Navarkos Guide](../README.md) to understand the basic concept and use cases.
- For Example 2, read [cluster-manager Guide](https://github.com/oracle/cluster-manager/blob/master/README.md) to understand the basic concept and use cases.

## Example 1: Navarkos without cluster-manager

Navarkos re-balances replicas allocation among clusters to optimize usage/load. To achieve this:

- Navarkos provides for specifying priority at a cluster level. This can be done by specifying annotation `n6s.io/cluster.priority` on a cluster object, with "1" being highest. If not specified, default priority is set to "1".
- Navarkos finds out what clusters are federated and how they are currently used for applications (load).
- Navarkos then re-balances replicas for the requested deployment in the prioritized group of cluster i.e. it tries to fulfill the demand of replicas, starting with clusters which are in highest priority group before assigning them to lower priority cluster(s).
- Navarkos provides for control on cluster load i.e. how much capacity must be (and can be) allocated for each cluster. This can be done by specifying annotation `n6s.io/cluster.autoscale.scale-up-threshold` and `n6s.io/cluster.autoscale.scale-down-threshold` on a cluster object. If not specified, it defaults to 80% and 10% respectively.

1. This example requires that you have atleast two federated clusters. You can manually create managed clusters (e.g. cluster1 and cluster2) and [join them to the Federation](https://kubernetes.io/docs/tasks/federation/set-up-cluster-federation-kubefed/#adding-a-cluster-to-a-federation).
	```
	kubefed join cluster1
	kubefed join cluster2
	```

2. Annotate one cluster2 with lower priority.
    ```
    kubectl --context=<your federation context> annotate cluster cluster2 n6s.io/cluster.priority="2"
    ```

3. Create a simple application deployment. For example, you can refer to this [file sample](helloworld-deployment.yaml).
	```
	kubectl create --context=<your federation context> namespace navarkos-test-app
	kubectl create --context=<your federation context> -f helloworld-deployment.yaml -n navarkos-test-app
	```

4. Verify the application's replica distribution at federation level.
    ```
    kubectl --context=<your federation context> get deployments helloworld -n navarkos-test-app
    ```

5. Verify the application's replica are distributed to top priority cluster only i.e. cluster1.
    ```
    kubectl --context=cluster1 get po --all-namespaces
    ```

    Cluster capacity varies by provider (and by configuration, in some cases) so its hard to predict exact distribution. However, we are requesting only "10" replicas, so in most cases, it should be on cluster1.

6. Find out cluster1's total capacity.
    ```
    kubectl --context=<your federation context> get clusters cluster1 -o yaml
    ```

    Examine the value for annotation `n6s.io/cluster.capacity.total-capacity-pods`

7. Update the replica count for [deployment](helloworld-deployment.yaml) such that its more than default scale up threshold i.e. more than 80% of cluster1's capacity and then update this deployment.
    ```
    kubectl --context=<your federation context> replace -f helloworld-deployment.yaml -n navarkos-test-app
    ```

8. Verify the application's replica distribution at federation and cluster level following instructions in step 4 and 5.

9. (Optional) Delete the application at the end.
	```
	kubectl delete --context=<your federation context> -f helloworld-deployment.yaml -n navarkos-test-app
	kubectl delete --context=<your federation context> namespace navarkos-test-app
	```
	
## Example 2: Navarkos with cluster-manager

When used in tandem with [cluster-manager](https://github.com/oracle/cluster-manager), Navarkos can re-balance demand by performing life cycle operations on clusters as well. That is, it can provision more capacity (Provision/Scale up) or removing excessive allocated capacity (Scale Down/Shutdown) of cluster, to re-balance application replicas. To achieve this:

- Deploy cluster-manager following [Deploy instructions](https://github.com/oracle/cluster-manager/blob/master/deploy/README.md).
- Navarkos updates `n6s.io/cluster.lifecycle.state` annotation on a cluster object to communicates the cluster life cycle state changes.

1. In this example, we don't have to have a federated cluster as Navarkos can use cluster-manager to provision one on demand. However, cluster must be created in repository for cluster objects so that cluster-manager can acts on it. Follow Step-1 of ["Provisioning * cluster" instructions](https://github.com/oracle/cluster-manager/blob/master/deploy/README.md). For example, if you follow "Provisioning an AWS Cluster" then cluster name will be "akube-us-east-2".

2. Create a simple application deployment. For example, you can refer to this [file sample](helloworld-deployment.yaml).
  	```
  	kubectl create --context=<your federation context> namespace navarkos-test-app
  	kubectl create --context=<your federation context> -f helloworld-deployment.yaml -n navarkos-test-app
  	```

3. Navarkos will notice that there is requirement for application deployment and communicate with cluster-manager to provision cluster to fulfill this demand by updating `n6s.io/cluster.lifecycle.state` with value `pending-provision` on a cluster created in step 1.

4. cluster-manager will then work on provisioning the cluster and change value for `n6s.io/cluster.lifecycle.state` to `ready` if successful or `failed-provision` if failed.

5. Verify that the cluster has been provisioned.
    ```
    kubectl --context=<your federation context> get clusters
    ```

6. Verify the application's replica distribution at federation level.
    ```
    kubectl --context=<your federation context> get deployments helloworld -n navarkos-test-app
    ```

7. Verify the application's replica are distributed to top priority cluster only i.e. cluster1.
    ```
    kubectl --context=akube-us-east-2 get po --all-namespaces
    ```

    Cluster capacity varies by provider (and by configuration, in some cases) so its hard to predict exact distribution. However, we are requesting only "10" replicas, so in most cases, it should be on "akube-us-east-2".

8. (Optional) Delete the application at the end.
	```
	kubectl delete --context=<your federation context> -f helloworld-deployment.yaml -n navarkos-test-app
	kubectl delete --context=<your federation context> namespace navarkos-test-app
	```