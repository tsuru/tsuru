.. Copyright 2013 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

+++++++++++++++++++
Build your own PaaS
+++++++++++++++++++

This document describes how to create a private PaaS service using tsuru. It
contains instructions on how to build tsuru and some of its components from
source.

This document assumes that tsuru is being installed on a Ubuntu machine. You
can use equivalent packages for beanstalkd, git, MongoDB and other tsuru
dependencies. Please make sure you satisfy minimal version requirements.

There's also a contributed `Vagrant <http://www.vagrantup.com/>`_ box, that
setups a PaaS using `Chef <http://www.opscode.com/chef/>`_. You can check this
out: https://github.com/hfeeki/vagrant-tsuru.

Overview
========

The Tsuru PaaS is composed by multiple components:

* tsuru webserver
* tsuru collector
* gandalf
* charms

And these components have their own dependencies, like:

* mongodb (>=2.2.0)
* beanstalkd (>=1.4.6)
* git-daemon (git>=1.7)
* juju (python version, >=0.5)
* libyaml (>=0.1.4)

Installing tsuru's dependencies
===============================

Using apt-get, that's an easy task:

.. highlight:: bash

    $ sudo apt-get install git beanstalkd libyaml-dev juju

You will also need MongoDB stable, distributed by 10gen. `It's pretty easy to
get it running on Ubuntu <ec2-50-19-178-134.compute-1.amazonaws.com>`_.

Please make sure that you `configure Juju
<https://juju.ubuntu.com/docs/getting-started.html#configuring-your-environment-using-ec2>`_
properly. Then run:

.. highlight:: bash

    $ juju bootstrap

Installing pre-built binaries
=============================

You can download pre-built binaries of tsuru and collector. There are binaries
available only for Linux 64 bits, so make sure that ``uname -m`` prints
``x86_64``:

.. highlight:: bash

::

    $ uname -m
    x86_64

Then download and install the binaries. First, collector:

.. highlight:: bash

::

    $ curl -sL https://s3.amazonaws.com/tsuru/dist-server/tsuru-collector.tar.gz | sudo tar -xz -C /usr/bin

Then the API webserver:

.. highlight:: bash

::

    $ curl -sL https://s3.amazonaws.com/tsuru/dist-server/tsuru-api.tar.gz | sudo tar -xz -C /usr/bin

These commands will install ``collector`` and ``api`` commands in ``/usr/bin``
(you will need to be a sudoer and provide your password). You may install these
commands somewhere else in your ``PATH``.

Installing from source
======================

1. Install the build requirements for tsuru.

.. highlight:: bash

::

    $ sudo apt-get install -y golang-go git mercurial bzr gcc

2. Build and install tsuru webserver and collector

.. highlight:: bash

::

    $ export GOPATH=/home/ubuntu/.go
    $ export PATH=${GOPATH}/bin:${PATH}
    $ go get github.com/globocom/tsuru/api
    $ go get github.com/globocom/tsuru/collector

Configuring tsuru
=================

Before running tsuru, you must configure it. By default, tsuru will look for
the configuration file in the ``/etc/tsuru/tsuru.cnf`` path. You can check a
sample configuration file and documentation for each tsuru setting in the
:doc:`"Configuring tsuru" </config>` page.

You can download the sample configuration file from Github:

.. highlight:: bash

::

    $ [sudo] mkdir /etc/tsuru
    $ [sudo] curl -sL https://raw.github.com/globocom/tsuru/master/etc/tsuru.conf -o /etc/tsuru/tsuru.conf

Make sure you define the required settings (database connection, authentication
configuration, AWS credentials, etc.) before running tsuru.

Running tsuru
=============

Now that you have ``api`` and ``collector`` properly installed, and you
:doc:`configured tsuru </config>`, you're four steps away from running it.

1. Start mongodb

.. highlight:: bash

::

    $ sudo service mongodb  start

2. Start beanstalkd

.. highlight:: bash

::

    $ sudo service beanstalkd start

3. Download the charms

Charms define how platforms will be installed. To use the charms provided by
tsuru you can get it from `tsuru charms repository
<https://github.com/globocom/charms>`_ and put it somewhere. Then define the
setting ``juju:charms-path`` in the configuration file:

.. highlight:: bash

::

    $ git clone git://github.com/globocom/charms.git /home/me/charms
    $ cat /etc/tsuru/tsuru.conf
    # ...
    juju:
      charms-path: /home/me/charms

4. Starting tsuru and collector

.. highlight:: bash

::

    $ api &
    $ collector &
