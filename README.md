#tsuru

[![Build Status](https://travis-ci.org/tsuru/tsuru.png?branch=master)](https://travis-ci.org/tsuru/tsuru)
[![Go Report Card](https://goreportcard.com/badge/github.com/tsuru/tsuru)](https://goreportcard.com/report/github.com/tsuru/tsuru)

##What is tsuru?

tsuru is an extensible and open source Platform as a Service (PaaS) that makes
application deployments faster and easier.
tsuru is an open source polyglot cloud application platform (PaaS).
With tsuru, you don’t need to think about servers at all.
As an application developer, you can:

- Write apps in the programming language of your choice,
- Back apps with add-on resources such as SQL and NoSQL databases, including memcached, redis, and many
others.
- Manage apps using the ``tsuru`` command-line tool
- Deploy apps using the Git revision control system

Links:

- Full Documentation: https://docs.tsuru.io
- How to Contribute: https://docs.tsuru.io/stable/contributing/
- Repository & Issue Tracker: https://github.com/tsuru/tsuru
- Talk to us on Gitter: https://gitter.im/tsuru/tsuru

## Quick Start 

With the purpose of test tsuru and/or for development, you can use [installer](https://docs.tsuru.io/master/experimental/installer.html) to have tsuru up and running. Installer is an experimental feature. 

### From Binary

#### Getting tsuru-client 

Download the latest release for your platform at: https://github.com/tsuru/tsuru-client/releases/

Example for release 1.1.1 and OS X.

```
$ curl -sSL https://github.com/tsuru/tsuru-client/releases/download/1.1.1/tsuru-1.1.1-darwin_amd64.tar.gz \
  | tar xz
```

#### Call tsuru installer

```
$ tsuru install
```

### From Source

#### Getting tsuru-client 

You need to have [Go](https://golang.org/doc/install) properly installed in your machine.

```
$ git clone github.com/tsuru/tsuru-client $GOPATH/src/github.com/tsuru/tsuru-client
$ cd $GOPATH/src/github.com/tsuru/tsuru-client
$ make install
```

#### Create an installer config

Create a file called local.yml with this content:

```
components:
    tsuru:
        version: latest
```

#### Call tsuru installer

```
$ tsuru install -c local.yml
```

### Testing

If everything's gone well you have the tsuru running on a virtualbox. 
Call app-list to see tsuru working, this command needs to return one app called tsuru-dashboard.

```
$ tsuru app-list
```
