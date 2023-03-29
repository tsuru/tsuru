# Installing Tsuru locally

This guide will walk you through running the tsuru api locally and attaching it to your running kubernetes cluster

## Prerequisites

1. [Minikube](https://minikube.sigs.k8s.io/docs/start/) (this can be optional if you already have a cluster running)
2. [kubectl](https://kubernetes.io/docs/tasks/tools/)

## Git clone

First things thirst, please clone tsuru repository and _cd_ into it:

`git clone https://github.com/tsuru/tsuru.git & cd tsuru`

## Jumpstarting the cluster (optional)

Tsuru has a Makefile recipe `make local` to locally run a minikube cluster on your machine, but feel free to bootstrap your cluster anyway you want.

> It's important to make sure the tsuru api has access to the services running inside the cluster and can talk to the Kubernetes api, this can be easily achieved by running
> the cluster using the _none_ driver when running minikube
>
> If you just want to test things out locally we highly recommend the usage of `make local` for simplicity and support

## Run the tsuru api

* If you've ran the `make local` recipe and got no errors the api should already be up and running.

    > If you decide to create the cluster on your behalf, you still have to build and run the api, there's also a recipe for that:
`make local-api`

## Admin Configuration

* Edit the _host_ paramater inside _etc/tsuru-local.conf_ and replace it with your own local IP

    > To find out your local IP either run `ifconfig` or the newer `ip --addr` command

* Create the admin user on tsuru:

      `tsurud root user create admin@admin.com`

      > You can replace `admin@admin.com` with any email you'd prefer

* Add the localhost to tsuru target and log in using your credentials created in step 1:

      `tsuru target-add default https://localhost:8080 -s`

      `tsuru login`

* Create a team:

    `tsuru team create admin-team`

* Create a pool:

    `tsuru pool create kubepool --provisioner=kubernetes --default=true`

* Add your running cluster to tsuru:

    ```tsuru cluster add minikube kubernetes --addr https://`minikube ip`:8443 --cacert $HOME/.minikube/ca.crt --clientcert $HOME/.minikube/profiles/minikube/apiserver.crt --clientkey $HOME/.minikube/profiles/minikube/apiserver.key --pool kubepool```

* Check your node IP:

    `tsuru node list -f tsuru.io/cluster=minikube`

* Register your node IP as a member of kubepool:

    `tsuru node update <node ip> pool=kubepool`

## Optional Steps

* Add a platform:

      `tsuru platform add go`

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
