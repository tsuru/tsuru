.. Copyright 2013 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

++++++++++++++++++++++++++++++++++++++++++++
Setting up you tsuru development environment
++++++++++++++++++++++++++++++++++++++++++++

To install tsuru from source, you need to have Go installed and configured.
This file will guide you through the necessary steps to get tsuru's development
environment.

Installing Go
=============

You need to install the last version of Go to compile tsuru. You can download
binaries distribution from `Go website <http://golang.org/doc/install>`_ or use
your preferred package installer (like Homebrew on Mac OS and apt-get on
Ubuntu):

.. highlight:: bash

::

    $ [sudo] apt-get install golang

    $ brew install go


Installing MongoDB
==================

tsuru uses MongoDB (+2.2), so you need to install it. For that, you can follow
instructions on MongoDB website and download binary distributions
(http://www.mongodb.org/downloads). You can also use your preferred package
installer:

.. highlight:: bash

::

    $ sudo apt-key adv --keyserver keyserver.ubuntu.com --recv 7F0CEB10
    $ sudo bash -c 'echo "deb http://downloads-distro.mongodb.org/repo/ubuntu-upstart dist 10gen" > /etc/apt/sources.list.d/10gen.list'
    $ sudo apt-get update
    $ sudo apt-get install mongodb-10gen -y

    $ brew install mongodb

Installing Beanstalkd
=====================

Tsuru uses `Beanstalkd <http://kr.github.com/beanstalkd/>`_ as a work queue.
Install the latest version, by doing this:

.. highlight:: bash

::

    $ sudo apt-get install -y beanstalkd

    $ brew install beanstalkd

Installing Redis
================

One of Tsuru routing providers uses `Redis <http://redis.io>`_ to store
information about frontends and backends. You will also need to install it:

.. highlight:: bash

::

    $ sudo apt-get install -y redis-server

    $ brew install redis

Installing git, bzr and mercurial
=================================

tsuru depends on go libs that use git, bazaar and mercurial, so you need to install
these two version control systems to get and compile tsuru from source.

To install git, you can use your package installer:

.. highlight:: bash

::

    $ sudo apt-get install git

    $ brew install git

To install bazaar, follow the instructions in bazaar's website
(http://wiki.bazaar.canonical.com/Download), or use your package installer:

.. highlight:: bash

::

    $ sudo apt-get install bzr

    $ brew install bzr

To install mercurial, you can also follow instructions on its website
(http://mercurial.selenic.com/downloads/) or use your package installer:

.. highlight:: bash

::

    $ sudo apt-get install mercurial

    $ brew install mercurial


Setting up GOPATH and cloning the project
=========================================

Go uses an environment variable called GOPATH to allow users to develop using
the go build tool (http://golang.org/cmd/go). So you need to setup this
variable before cloning and installing tsuru. You can set this variable to your
$HOME directory, or something like `$HOME/gocode`.

Once you have defined the GOPATH variable, then run the following commands:

.. highlight:: bash

::

    $ mkdir -p $GOPATH/src/github.com/globocom
    $ cd $GOPATH/src/github.com/globocom
    $ git clone git://github.com/globocom/tsuru

If you have already cloned the repository, just move the cloned directory to
`$GOPATH/src/github.com/globocom`.

For more details on GOPATH, please check this url:
http://golang.org/cmd/go/#GOPATH_environment_variable

Starting Redis, Beanstalkd and MongoDB
======================================

Before building the code and running the tests, execute the following commands 
to start Redis, Beanstalkd and MongoDB processes.

.. highlight:: bash

::

    $ redis-server
    $ mongod
    $ beanstalkd -l 127.0.0.1

Installing tsuru dependencies and running tests
===============================================

You can use `make` to install all tsuru dependencies and run tests. It will
also check if everything is ok with your GOPATH setup:

.. highlight:: bash

::

    $ make
