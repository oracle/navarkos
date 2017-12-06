# Getting Started

These instructions will get you a copy of the project up and running on your local machine for development and testing purposes.

## Prerequisites

- You must have [Federation](https://kubernetes.io/docs/tasks/federation/set-up-cluster-federation-kubefed/) set up to deploy Navarkos. It should be using on Kubernetes v1.8, atleast, for both server and clients.
- Navarkos is installed on the Federation host, so set your *kubectl* config context to the Federation host context.
	```
	kubectl config use-context <your federation host cluster>
	```

## Deploy
You can deploy Navarkos either by using yaml or helm.

### Using yaml

One way of deploying Navarkos is to use [`navarkos.yaml`](./kubectl/navarkos.yaml).

1. Override the defaults in ./kubectl/env.sh with required environment variables.
    * `FED_API`: set to your Federation API server endpoint. For example, `https://change.me`.
    * `FED_CONTEXT`: set to your Federation name. Defaults to `federation`, if not set.
    * `FED_NS`: set to the namespace where Federation Controller Plane was started via `kubefed init`. Defaults to `federation-system`, if not set.
    * `NAVARKOS_IMAGE`: set to full path where built Navarkos image is published. For example, `docker.io/changeme/navarkos:0.1.0v`.

2. Install Navarkos (requires envsubst to be available on PATH).
    ```
    cd kubectl
    source env.sh
    envsubst < navarkos.yaml | kubectl apply -f -
    ```

3. Verify if Navarkos is installed.
    ```
    kubectl get pods --all-namespaces | grep navarkos
    ```

4. (Optional) Uninstall Navarkos for later deployment.
    ```
    kubectl delete deployment navarkos -n $FED_NS
    ```

### Using Helm

1. Install tiller if not installed on the Federation host.
    ```
    helm init
    ```

2. Override the defaults in ./helm/navarkos/values.yaml for the helm chart.
    * `federationEndpoint`: set to your Federation API server endpoint. For example, `https://change.me`.
    * `federationContext`: set to your Federation name. Defaults to `federation`, if not set.
    * `federationNamespace`: set to the namespace where Federation Controller Plane was started via `kubefed init`. Defaults to `federation-system`, if not set.
    * `image.repository`: set to the `respository` where built Navarkos image is published.
    * `image.tag`: set to the `tag` of published Navarkos image.

3. Install Navarkos.
    ```
    helm install ./helm/navarkos -n navarkos
    ```

4. Verify if Navarkos is installed.
    ```
    helm list navarkos
    ```

5. (Optional) Uninstall Navarkos for later deployment.
    ```
    helm delete --purge navarkos
    ```