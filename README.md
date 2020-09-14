# tsuru

[![Build Status](https://github.com/tsuru/tsuru/workflows/ci/badge.svg?branch=master)](https://github.com/tsuru/tsuru/actions)
[![Go Report Card](https://goreportcard.com/badge/github.com/tsuru/tsuru)](https://goreportcard.com/report/github.com/tsuru/tsuru)

## What is tsuru?

tsuru is an extensible and open source Platform as a Service (PaaS) that makes application deployments faster and easier.
With tsuru, you don’t need to think about servers at all. As an application developer, you can:
- Write apps in the programming language of your choice
- Back apps with add-on resources such as SQL and NoSQL databases, including memcached, Redis, and many others
- Manage apps using the `tsuru` command-line tool
- Deploy apps using the Git version control system

Links:

- Full Documentation: https://docs.tsuru.io
- How to Contribute: https://docs.tsuru.io/stable/contributing/
- Repository & Issue Tracker: https://github.com/tsuru/tsuru
- Talk to us on Gitter: https://gitter.im/tsuru/tsuru

## Quick Start

With the purpose of testing tsuru and/or for development, you can use the [installer](https://docs.tsuru.io/stable/installing/using-tsuru-installer.html) to have tsuru up and running. The installer is an experimental feature.

### From Binary

#### Getting tsuru-client

Download the latest release for your platform at: https://github.com/tsuru/tsuru-client/releases/

Example for release `1.1.1` and `OS X`:

```
$ curl -sSL https://github.com/tsuru/tsuru-client/releases/download/1.1.1/tsuru-1.1.1-darwin_amd64.tar.gz | tar xz
```

#### Call tsuru installer

```
$ tsuru install create
```

### From Source

#### Getting tsuru-client

You need to have [Go](https://golang.org/doc/install) properly installed on your machine.

```
$ git clone https://github.com/tsuru/tsuru-client $GOPATH/src/github.com/tsuru/tsuru-client
$ cd $GOPATH/src/github.com/tsuru/tsuru-client
$ make install
```

#### Create an installer config

Create the tsuru installer config files with:

```
$ tsuru install config init
```

Replace the tsuru API image tag with the latest tag in `install-compose.yml`:

```
$ sed -i'' -e 's/api:v1/api:latest/g' install-compose.yml
```

#### Call tsuru installer

```
$ $GOPATH/bin/tsuru install create -c install-config.yml -e install-compose.yml
```

### Testing

If everything's gone well you have the tsuru running in a VirtualBox VM.
Call `app-list` to see tsuru working, this command needs to return one app called tsuru-dashboard.

```
$ tsuru app-list
```
