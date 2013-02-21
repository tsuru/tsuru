.. Copyright 2013 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

+++++++++++++++++++
Build your own PaaS
+++++++++++++++++++

This documents describes how to create a private PaaS service using tsuru.
We will be building the PaaS using source avaliable at https://github.com/globocom/tsuru.

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

Installing tsuru webserver from source
======================================

1. Install the base requirements for tsuru.

.. highlight:: bash

::

    $ sudo apt-get update
    $ sudo apt-get upgrade
    $ sudo apt-get install -y golang-go git mercurial bzr gcc beanstalkd

Tsuru needs the mongodb version 2.2>= so, to install it please `do this simple steps <http://docs.mongodb.org/manual/tutorial/install-mongodb-on-ubuntu/>`_

Tsuru uses juju to orchestrates your "apps". To install juju follow the `juju install guide <https://juju.ubuntu.com/docs/getting-started.html#installation>`_. It's need to configure the `.juju/enviroment.yml` and do the `bootstrap` too.

2. Install the tsuru api

.. highlight:: bash

::

    $ export GOPATH=/home/ubuntu/.go
    $ export PATH=${GOPATH}/bin:${PATH}
    $ go get github.com/globocom/tsuru/api
    $ go get github.com/globocom/tsuru/collector

3. Start mongodb

.. highlight:: bash

::

    $ sudo service mongodb  start

4. Start beanstalkd

.. highlight:: bash

::

    $ sudo service beanstalkd start


5. Configuring tsuru

.. highlight:: bash

::

    $ sudo mkdir -p /etc/tsuru
    $ sudo touch /etc/tsuru/tsuru.conf

6. Download the charms

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

7. Starting tsuru and collector

.. highlight:: bash

::

    $ api &
    $ collector &
