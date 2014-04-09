.. Copyright 2014 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

This document describes the installation of a single docker node. It can be use to create
a docker cluster to be used by tsuru. At the end of this document, you will have a running
and configured docker node.

Setup
-----

This document assumes you already have a tsuru server installed and running, if you don't, follow the
:doc: `installation with docker <docker>`. The docker installation on tsuru server
in this case is optional, if you install it in the end you'll have two docker nodes.

Now that you have you setup ready, let's see how to install docker:

docker
------

.. highlight:: bash

::

    $ wget -qO- https://get.docker.io/gpg | sudo apt-key add -
    $ echo 'deb http://get.docker.io/ubuntu docker main' | sudo tee /etc/apt/sources.list.d/docker.list
    $ sudo apt-get update
    $ sudo apt-get install lxc-docker

Then edit ``/etc/init/docker.conf`` to start docker on tcp://0.0.0.0:4243:

.. highlight:: bash

::

    description     "Docker daemon"

    start on filesystem and started lxc-net
    stop on runlevel [!2345]

    respawn

    script
        /usr/bin/docker -H tcp://0.0.0.0:4243 -d
    end script

Now start it:

.. highlight:: bash

::

    $ sudo start docker

tsuru node agent
----------------

Now that you have docker installed and running it's time to install tsuru node agent.
This agent is responsible to announce a docker node, unnannounce it, and run the ``docker-ssh-agent``.

Add the tsuru/ppa then install it.

.. highlight:: bash

::

    $ sudo add-apt-repository -y ppa:tsuru/ppa
    $ sudo apt-get update
    $ sudo apt-get tsuru-node-agent

Start ``docker-ssh-agent``:

.. highlight:: bash

::

    $ start tsuru-node-agent docker-ssh-agent

Now it is need to announce the node we just created:

.. highlight:: bash

::

    $ tsuru-node-agent node-add address=<address> ID=<server-id> team=<team-owner> -h <tsuru-api:port>

The ``address`` parameter is the address of the node we just installed.
The ``ID`` parameter is the identifier for this host that tsuru will use.
The ``team`` parameter is the team that this host will attend, see the scheduler docs to know more about it. This
parameter is optional, if not passed, this node will be host of teams that doesn't have a node associated for them.
The ``-h`` flag is mandatory, and should be passed in the form http://tsuruhost.com, with http (or https),
otherwise it won't be accepted as valid.

It's highly important that every node is announced using the ``node-add`` command. The configuration ``docker:servers`` on
tsuru.conf is deprecated and will be removed.
