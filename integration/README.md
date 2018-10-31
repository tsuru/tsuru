# Integration tests

Package integration provides a set of integration tests for tsuru. The testing behavior
can be configured by a set of environment variables.

These tests might be ran on an existing tsuru installation (by providing the correct amount
of environment variables) or it can test a new installation (locally or in a cloud) by
leveraging tsuru installer.

## Basics

The integration testing consists of a sequence of steps defined as the type:

	ExecFlow{}

These steps are registered in the global flows variable. Each step has a provides
field which registers what resources this step provides, and will only run if this resource is
not provided by an environment variable `TSURU_INTEGRATION_<provides>`. They also might require specific
resources by setting the requires field.

## Running locally

` $ make test-int`

## Examples

- Amazon Web Services - https://github.com/tsuru/integration_ec2
- Google Cloud Engine - https://github.com/tsuru/integration_gce
- Local -	https://github.com/tsuru/tsuru/blob/master/Makefile#L138

## Environment Variables

The following variables may be set to customize the integration testing, these are all prefixed with
TSURU_INTEGRATION_.

### General

- enabled - enables integration testing
- maxconcurrency - test concurrency
- verbose - test verbosity
- examplesdir - path to the platforms examples
- nodeopts - Additional options passed to node creation.
- installername - Name of the installation to be created.
- provisioners - List of provisioners to test. Defaults to docker.
- clusters - List of cluster providers. Defaults to swarm. Available values are gce, swarm and minikube.
- platforms - List of platforms to test. Defaults to all platforms available on https://github.com/tsuru/platforms

### Flow control

- platformimages
- installerconfig
- installercompose
- targetaddr
- team
- poolnames
- installedplatforms
- serviceimage
- servicename

### Cluster configuration

#### gce

- clustername - If provided, instead of provisioning a kubernetes cluster in gce, the one with this name will be used.
- GOOGLE_APPLICATION_CREDENTIALS - path to a file containing gce credentials
- GCE_ZONE - the [GCE zone](https://cloud.google.com/compute/docs/regions-zones/regions-zones) where you want the Tsuru instance to be created
- GCE_PROJECT_ID - ID for a project created in GCE
- GCE_MACHINE_TYPE - Machine type to be used for the kubernetes cluster master
- GCE_SERVICE_ACCOUNT - GCE service account name
