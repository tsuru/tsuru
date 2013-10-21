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

* tsuru server
* tsuru collector
* gandalf
* charms

And these components have their own dependencies, like:

* mongodb (>=2.2.0)
* beanstalkd (>=1.4.6)
* git-daemon (git>=1.7)
* juju (python version, >=0.5)
* libyaml (>=0.1.4)

Requirements
============

1. Operating System
-------------------

The steps below will guide you throught the install process on Ubuntu Server
12.04.

If you try to build tsuru server on most Linux systems, you should have few
problems and if there are problems, we are able to help you. Just
ask on #tsuru channel on irc.freenode.net.

* *Have you tried tsuru server on other systems? Let us know
  and* :doc:`contribute </community>` *to the project.*

2. Hardware
-----------

Tsuru server is a lightweight framework and can be run in a single small machine along with all the deps.

3. Software
-----------

**3.1 MongoDB**

Tsuru needs MongoDB stable, distributed by 10gen. `It's pretty easy to
get it running on Ubuntu <http://docs.mongodb.org/manual/tutorial/install-mongodb-on-ubuntu/>`_

**3.2 Juju**

Tsuru uses juju to orchestrates your "apps". To install juju follow the `juju
install guide
<https://juju.ubuntu.com/docs/getting-started.html#installation>`_.
Please make sure that you `configure Juju
<https://juju.ubuntu.com/docs/getting-started.html#configuring-your-environment-using-ec2>`_
properly. Then run:

.. highlight:: bash

::

    $ juju bootstrap

Juju Charms define how platforms will be installed.  You may take a look at
`juju charms collection <http://jujucharms.com/charms>`_ or use the `charms
provided by tsuru <https://github.com/globocom/charms>`_

Put it somewhere and define the setting ``juju:charms-path`` in the configuration
file:

.. highlight:: bash

::

    $ git clone git://github.com/globocom/charms.git /home/me/charms
    $ cat /etc/tsuru/tsuru.conf
    # ...
    juju:
      charms-path: /home/me/charms

**3.3 Beanstalkd**

Tsuru uses `Beanstalkd <http://kr.github.com/beanstalkd/>`_ as a work queue.
Install the latest version, by doing this:

.. highlight:: bash

::

    $ sudo apt-get install -y beanstalkd

**3.4 Gandalf**

Tsuru uses `Gandalf <https://github.com/globocom/gandalf>`_ to manage git
repositories, to get it installed `follow this steps
<https://gandalf.readthedocs.org/en/latest/install.html>`_

Installing from PPA
===================

You can use ``apt-get`` to install Gandalf using `Tsuru's ppa
<https://launchpad.net/~tsuru/+archive/ppa>`_:

.. highlight:: bash

::

    $ sudo apt-add-repository ppa:tsuru/ppa -y
    $ sudo apt-get update
    $ sudo apt-get install tsuru-server

Then you will need to edit the file ``/etc/default/tsuru-server`` and enable the API and the colletor:

.. highlight:: bash

::

    TSR_API_ENABLED=yes
    TSR_COLLECTOR_ENABLED=yes

Make sure you edit the configuration file (see `Configuring tsuru`_) and then
start API and collector using upstart:

.. highlight:: bash

::

    $ sudo start tsuru-server-api
    $ sudo start tsuru-server-collector

Installing pre-built binaries
=============================

You can download pre-built binaries of tsuru and collector. There are binaries
available only for Linux 64 bits, so make sure that ``uname -m`` prints
``x86_64``:

.. highlight:: bash

::

    $ uname -m
    x86_64

Then download and install the tsr binary:

.. highlight:: bash

::

    $ curl -sL https://s3.amazonaws.com/tsuru/dist-server/tsr-master.tar.gz | sudo tar -xz -C /usr/bin


These commands will install ``tsr`` in ``/usr/bin``
(you will need to be a sudoer and provide your password). You may install this
command in your ``PATH``.

Installing from source
======================

0. Build dependencies

To build tsuru from source you will need to install the following packages

.. highlight:: bash

::

    $ sudo apt-get install -y golang-go git mercurial bzr gcc

1. Install the tsuru tsr

Add the following lines to your ~/.bashrc:

.. highlight:: bash

::

    $ export GOPATH=/home/ubuntu/.go
    $ export PATH=${GOPATH}/bin:${PATH}

Then execute:

.. highlight:: bash

::

    $ source ~/.bashrc
    $ go get github.com/globocom/tsuru/tsr

Configuring tsuru
=================

Before running tsuru, you must configure it. By default, tsuru will look for
the configuration file in the ``/etc/tsuru/tsuru.conf`` path. You can check a
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

Now that you have ``tsr`` properly installed, and you
:doc:`configured tsuru </config>`, you're three steps away from running it.

1. Start mongodb

.. highlight:: bash

::

    $ sudo service mongodb  start

2. Start beanstalkd

.. highlight:: bash

::

    $ sudo service beanstalkd start

3. Start api and collector

.. highlight:: bash

::

    $ tsr api &
    $ tsr collector &

One can see the logs in:

.. highlight:: bash

::

    $ tail -f /var/log/syslog

Using tsuru
===========

Congratulations! At this point you should have a working tsuru server running
on your machine, follow the :doc:`tsuru client usage guide
</apps/client/usage>` to start build your apps.
