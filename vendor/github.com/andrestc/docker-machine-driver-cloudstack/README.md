# Docker Machine CloudStack Driver

Docker Machine CloudStack Driver is a driver for [Docker Machine](https://docs.docker.com/machine/).
It allows to create Docker hosts on [Apache CloudStack](https://cloudstack.apache.org/) and
[Accelerite CloudPlatform](http://cloudplatform.accelerite.com/).

## Requirements

* [Docker Machine](https://docs.docker.com/machine/) 0.5.1 or later

## Installation

Download the binary from follwing link and put it within your PATH (ex. `/usr/local/bin`)

https://github.com/andrestc/docker-machine-driver-cloudstack/releases/latest

## Usage

```bash
docker-machine create -d cloudstack \
  --cloudstack-api-url CLOUDSTACK_API_URL \
  --cloudstack-api-key CLOUDSTACK_API_KEY \
  --cloudstack-secret-key CLOUDSTACK_SECRET_KEY \
  --cloudstack-template "Ubuntu Server 14.04" \
  --cloudstack-zone "zone01" \
  --cloudstack-service-offering "Small" \
  --cloudstack-expunge \
  docker-machine
```

## Acknowledgement

The driver is originally written by [@svanharmelen](https://github.com/svanharmelen) and [@atsaki](https://github.com/atsaki).
