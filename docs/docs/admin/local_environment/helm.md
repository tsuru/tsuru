# Installing Tsuru in a local Kubernetes cluster with Helm

All steps in this guide were done in Kubernetes v1.20.0. While it might work for almost all Kubernetes versions, some versions may break something. Feel free to report us in that case.

## Prerequisites

1. [Minikube](https://minikube.sigs.k8s.io/docs/start/)
2. [kubectl](https://kubernetes.io/docs/tasks/tools/)
3. [Helm](https://helm.sh/docs/intro/install/)

## Jumpstarting the cluster

Tsuru has a Makefile recipe (`make local`) to locally run a minikube cluster on your machine, but feel free to bootstrap your minikube cluster anyway you want.

## Hardware Requirements

* 2 CPUs or more
* 12GB of free memory
* 20GB of free disk space

## Installing the helm chart

* Adding tsuru's helm chart
      * `helm repo add tsuru https://tsuru.github.io/charts`  

* Installing the chart
      * `helm install tsuru tsuru/tsuru-stack --create-namespace --namespace tsuru-system`

**_Congratulations you have tsuru installed and runnning on your cluster!_**

## Admin Configuration

1. Create the admin user on tsuru:

      `kubectl exec -it -n tsuru-system deploy/tsuru-api -- tsurud root user create admin@admin.com`

      > You can replace `admin@admin.com` with any email you'd prefer

2. Use Port-forward to access tsuru and ingress controller locally:

    ```kubectl port-forward --namespace tsuru-system svc/tsuru-api 8080:80 &
    kubectl port-forward --namespace tsuru-system svc/tsuru-ingress-nginx-controller 8890:80 &
   ```

      > If you specified a port when you installed helm it will have to use the same port in tsuru-ingress-nginx-controller.

3. Add the localhost to tsuru target and log in:

      `tsuru target-add default https://localhost:8080 -s`

      `tsuru login`

4. Create a team:

      `tsuru team create admin`

5. Add at least one platform:

      `tsuru platform add go`

      > This is just an example, you could add python or other platforms i.e:
      >
      > `tsuru platform add python`

## Optional Steps

* Create and Deploy tsuru-dashboard app:

      `tsuru app create dashboard`

      `tsuru app deploy -a dashboard --image tsuru/dashboard`

* Create an app to test:

      `mkdir example-go`
   
      `cd example-go`

      `git clone https://github.com/tsuru/platforms.git`

      `cd platforms/examples/go`

      `tsuru app create example-go go`

      `tsuru app deploy -a example-go`
