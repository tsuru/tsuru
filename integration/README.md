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

` $ make test-integration`



## Environment Variables

The following variables may be set to customize the integration testing, these are all prefixed with
TSURU_INTEGRATION_.

### General

- enabled - enables integration testing
- maxconcurrency - test concurrency
- verbose - test verbosity
- examplesdir - path to the platforms examples
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

