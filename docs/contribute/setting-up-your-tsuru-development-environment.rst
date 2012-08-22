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

tsuru uses MongoDB, so you need to install it. For that, you can follow
instructions on MongoDB website and download binary distributions
(http://www.mongodb.org/downloads). You can also use your preferred package
installer:

.. highlight:: bash

::

    $ sudo apt-get install libyaml-dev

    $ brew install libyaml

Installing bzr and mercurial
============================

tsuru depends on go libs that use bazaar and mercurial, so you need to install
these two version control systems to get and compile tsuru from source.

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

    $ mkdir -p $GOPATH/src/github.com/timeredbull
    $ cd $GOPATH/src/github.com/timeredbull
    $ git clone git://github.com/timeredbull/tsuru

If you have already cloned the repository, just move the cloned directory to
`$GOPATH/src/github.com/timeredbull`.

For more details on GOPATH, please check this url:
http://golang.org/cmd/go/#GOPATH_environment_variable

Installing tsuru dependencies and running tests
===============================================

You can use `make` to install all tsuru dependencies and run tests. It will
also check if everything is ok with your GOPATH setup:

.. highlight:: bash

::

    $ make
