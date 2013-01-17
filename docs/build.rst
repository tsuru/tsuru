.. Copyright 2013 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

+++++++++++++++++++
Build your won PaaS
+++++++++++++++++++

This documents describes how to create a private PaaS service using tsuru.
We will be building the PaaS using source avaliable at https://github.com/globocom/tsuru.

Overview
========

The Tsuru PaaS is composed by X components:

* tsuru webserver
* tsuru collector
* gandalf
* charms

Installing tsuru webserver from source
======================================

1. Install the base requirements for tsuru.

.. highlight:: bash

::

    $ sudo apt-get update
    $ sudo apt-get upgrade
    $ sudo apt-get install -y golang-go git mercurial bzr gcc beanstalkd

Tsuru needs the mongodb version 2.2>= so, to install it please do this simple steps:
http://docs.mongodb.org/manual/tutorial/install-mongodb-on-ubuntu/

2. Install the tsuru api

.. highlight:: bash

::

    $ export GOPATH=/home/ubuntu/.go
    $ export PATH=${GOPATH}/bin:${PATH}
    $ go get github.com/globocom/tsuru/api

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
