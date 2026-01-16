# tsuru

[![Build Status](https://github.com/tsuru/tsuru/workflows/ci/badge.svg?branch=main)](https://github.com/tsuru/tsuru/actions)
[![Go Report Card](https://goreportcard.com/badge/github.com/tsuru/tsuru)](https://goreportcard.com/report/github.com/tsuru/tsuru)

## What is tsuru?

tsuru is an extensible and open source Platform as a Service (PaaS) that makes application deployments faster and easier.
With tsuru, you don’t need to think about servers at all. As an application developer, you can:
- Write apps in the programming language of your choice
- Back apps with add-on resources such as SQL and NoSQL databases, including memcached, Redis, and many others
- Manage apps using the `tsuru` command-line tool

Links:

- Landing page: https://tsuru.io
- Full Documentation: https://docs.tsuru.io/
- How to Contribute: https://docs.tsuru.io/contributing/docker-compose/
- Repository & Issue Tracker: https://github.com/tsuru/tsuru
- Talk to us on Gitter: https://gitter.im/tsuru/tsuru


Popular platforms supported:

- [Python](https://github.com/tsuru/platforms/tree/master/python)
- [Nodejs](https://github.com/tsuru/platforms/tree/master/nodejs)
- [GO](https://github.com/tsuru/platforms/tree/master/go)
- [Ruby](https://github.com/tsuru/platforms/tree/master/ruby)
- [PHP](https://github.com/tsuru/platforms/tree/master/php)
- [Perl](https://github.com/tsuru/platforms/tree/master/perl)
- [Lua](https://github.com/tsuru/platforms/tree/master/lua)
- [Java](https://github.com/tsuru/platforms/tree/master/java)

## Quick Start

### Getting tsuru-client

Download the latest release for your platform at: https://github.com/tsuru/tsuru-client/releases/

Example for release `1.1.1` and `OS X`:

```
$ curl -sSL https://github.com/tsuru/tsuru-client/releases/download/1.1.1/tsuru-1.1.1-darwin_amd64.tar.gz | tar xz
```

### Install Guides

* [Minikube](https://tsuru.github.io/docs/getting_started/install_minikube/)
* [GKE - Google Kubernetes Engine](https://tsuru.github.io/docs/getting_started/install_gke/)



### Testing

If everything's gone well you have the tsuru running in a Kubernetes Cluster.
Call `app list` to see tsuru working, this command needs to return one app called tsuru-dashboard.

```
tsuru app list
```

## Local development

### Dependencies

Before starting, make sure you have the following tools installed:

* [docker](https://docs.docker.com/engine/install) (or [podman](https://podman.io/docs/installation))
* [minikube](https://minikube.sigs.k8s.io/docs/start)
* [go](https://go.dev/dl/)
* [yq](https://github.com/mikefarah/yq#install)

You'll also need the [Tsuru Client](https://docs.tsuru.io/user_guides/install_client/) to interact with the Tsuru API.
If you haven't installed it yet, please do so.

**For macOS users**: We recommend using the **_qemu_** driver with **_socket_vmnet_** for Minikube clusters.
For more information on installing **_qemu_** and **_socket_vmnet_**, refer to the following links:

* [qemu](https://www.qemu.org/download/)
* [socket_vmnet](https://github.com/lima-vm/socket_vmnet)

**Note**: If you are using Docker-compatible alternatives like Podman, be sure to specify the `DOCKER` variable with the
correct binary when running make commands. For example: `make local.run DOCKER=podman`.

### Running local environment

To run the Tsuru API locally, you'll need to first set up the local environment.
This setup process is crucial because it creates the default configuration files, initializes required dependencies, and prepares your local system to host the Tsuru API.
The following command will handle all these tasks:

```bash
make local.setup
```

Once the setup is complete, you won’t need to run this command again unless you want to reset your environment.

After the initial setup, you can start the Tsuru API and its dependencies using the following command:

```bash
make local.run
```

Once the Tsuru API is running, open a new terminal window and configure your Tsuru CLI to point to the `local-dev` target.
This target tells the CLI to interact with your local Tsuru API instance rather than a remote server.
You can set the target using this command:

```bash
tsuru target-set local-dev
```

Tsuru's targets function similarly to Kubernetes' `kubectl` config contexts, allowing you to switch between different environments easily.

To confirm that everything is set up correctly, you can log in and list the clusters managed by your Tsuru API instance:

```bash
tsuru login admin@admin.com # password: admin@123
tsuru cluster list
```

If everything is working as expected, you should see your local Minikube cluster listed as the default provisioner.

### Creating an app or job

For that, you will have to create a team, pool and set a label to the minikube nodes to allow deploys on it

```bash
tsuru team create my-team
tsuru pool add my-pool

# make sure you are using the right kube config
kubectl label nodes minikube tsuru.io/pool=my-pool
```

### Running Integration tests

In order to run integration tests, you must:

1. Ensure that your local Tsuru API instance is up and running.
2. Create a Kubectl config file that **must not be** `$HOME/.kube/config` that points **only** to your Minikube cluster.
3. Run the integration tests using the following command:

```bash
INTEGRATION_KUBECONFIG=<your-minikube-kubeconfig> make local.test-ci-integration
```

### Cleaning up

When you're done working with your local environment, it's important to stop the services to free up system resources.
You can stop the dependencies using:

```bash
make local.stop
```

If you want to fully reset your environment, or if you no longer need the Tsuru API and its dependencies on your local machine, you can remove all associated resources using:

```bash
make local.cleanup
```
