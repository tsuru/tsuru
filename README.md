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

- Full Documentation: https://docs.tsuru.io/main/
- How to Contribute: https://docs.tsuru.io/stable/contributing/
- Repository & Issue Tracker: https://github.com/tsuru/tsuru
- Talk to us on Gitter: https://gitter.im/tsuru/tsuru


Popular plataforms supported:

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

* [Minikube](https://docs.tsuru.io/main/installing/installing-minikube.html)
* [GKE - Google Kubernetes Engine](https://docs.tsuru.io/main/installing/installing-gke.html)



### Testing

If everything's gone well you have the tsuru running in a Kubernetes Cluster.
Call `app list` to see tsuru working, this command needs to return one app called tsuru-dashboard.

```
$ tsuru app list
```
