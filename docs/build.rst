.. Copyright 2013 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

+++++++++++++++++++
Build your own PaaS
+++++++++++++++++++

This document describes how to create a private PaaS service using tsuru.  We
will be building the PaaS using source avaliable at
https://github.com/globocom/tsuru.

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

And these components have any dependencies, like:

* mongodb
* beanstalkd
* git daemon
* juju

Requirements
============

1. Operating System
-------------------

At the moment, tsuru webserver is fully supported and tested on Ubuntu 12.04 and
the steps below will guide you throught the install process.

If you try to build tsuru webserver on most Linux systems, you should have few
problems and if there are problems, we are able to help you. Just
ask on #tsuru channel on irc.freenode.net.

* *Have you tried tsuru webserver on other systems? Let us know 
  and* :doc:`contribute </community>` *to the project.*

2. Hardware
-----------

Memory: :TODO

CPU: :TODO

Disc: :TODO

3. Software
-----------

3.1 MongoDB

Tsuru needs the mongodb version 2.2>= so, to install it please `do this simple
steps <http://docs.mongodb.org/manual/tutorial/install-mongodb-on-ubuntu/>`_

3.2 Juju

Tsuru uses juju to orchestrates your "apps". To install juju follow the `juju
install guide
<https://juju.ubuntu.com/docs/getting-started.html#installation>`_.
It's need to configure the `.juju/enviroment.yml` and do the `bootstrap` too.

3.3 Beanstalkd

Tsuru uses `Beanstalkd <http://kr.github.com/beanstalkd/>`_ as a work queue.
Install the latest version, by doing this:

.. highlight:: bash

::

    $ sudo apt-get update && sudo apt-get upgrade
    $ sudo apt-get install -y beanstalkd

3.4 Build dependencies

To build tsuru from source you will need  to install the packages below

.. highlight:: bash

::

    $ sudo apt-get install -y golang-go git mercurial bzr gcc


Installing tsuru webserver from source
======================================

1. Install the tsuru api

Add the following lines to your ~/.bashrc:

.. highlight:: bash

::

    $ export GOPATH=/home/ubuntu/.go
    $ export PATH=${GOPATH}/bin:${PATH}

Then execute:

.. highlight:: bash

::

    $ source ~/.bashrc
    $ go get github.com/globocom/tsuru/api
    $ go get github.com/globocom/tsuru/collector

2. Configuring tsuru

.. highlight:: bash

::

    $ sudo mkdir -p /etc/tsuru
    $ sudo sh -c "curl -sL https://raw.github.com/globocom/tsuru/master/etc/tsuru.conf > /etc/tsuru/tsuru.conf"

Edit /etc/tsuru.conf as needed.

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
