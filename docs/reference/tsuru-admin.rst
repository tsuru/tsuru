.. Copyright 2014 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

+++++++++++++++++
tsuru-admin usage
+++++++++++++++++

tsuru-admin command supports administrative operations on a tsuru server.
Please make sure you have it :doc:`installed </using/install-client>` before
continuing on this guide.

In order to use ``tsuru-admin`` commands, a user should be an :ref:`admin user
<config_admin_user>`.  To be an admin user you should be member of an
:ref:`admin team <config_admin_team>`.

Setting a target
================

The target for the tsuru-admin command should point to the `listen` address
configured in your tsuru.conf file.

.. highlight:: yaml

::

    listen: ":8080"


.. highlight:: bash

::

    $ tsuru-admin target-add default tsuru.myhost.com:8080
    $ tsuru-admin target-set default

Commands
========

All the "container*"" commands below only exist when using the docker
provisioner.

.. _tsuru_admin_container_move_cmd:

container-move
--------------

.. highlight:: bash

::

    $ tsuru-admin container-move <container id> <to host>

This command allow you to specify a container id and a destination host, this
will create a new container on the destination host and remove the container
from its previous host.

.. _tsuru_admin_containers_move_cmd:

containers-move
---------------

.. highlight:: bash

::

    $ tsuru-admin containers-move <from host> <to host>

It allows you to move all containers from one host to another. This is useful
when doing maintenance on hosts. <from host> and <to host> must be host names
of existing docker nodes.

This command will go through the following steps:

* Enumerate all units at the origin host;
* For each unit, create a new unit at the destination host;
* Erase each unit from the origin host.

.. _tsuru_admin_containers_rebalance_cmd:

containers-rebalance
--------------------

.. highlight:: bash

::

    $ tsuru-admin containers-rebalance [--dry]

Instead of specifying hosts as in the containers-move command, this command
will automatically choose to which host each unit should be moved, trying to
distribute the units as evenly as possible.

The --dry flag runs the balancing algorithm without doing any real
modification. It will only print which units would be moved and where they
would be created.


All the "platform*"" commands below only exist when using the docker
provisioner.

.. _tsuru_admin_docker_node_add_cmd:

docker-node-add
---------------

.. highlight:: bash

::

    $ tsuru-admin docker-node-add [param_name=param_value]... [--register]

This command add a node to your docker cluster. By default, this command will
call the configured IaaS to create a new machine. Every param will be sent to
the IaaS implementation.

You should configure in **tsuru.conf** the protocol and port for IaaS be able
to access your node (`you can see it here <config.html#iaas-configuration>`_).

If you want to just register an docker node, you should use the --register
flag with an **address=http://your-docker-node:docker-port**

Parameters have special meaning
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

* ``iaas=<iaas name>`` Which iaas provider should be used, if not set tsuru will use
  the default iaas specified in tsuru.conf file.

* ``template=<template name>`` A machine template with predefined parameters,
  additional parameters will override template ones. See 
  :ref:`machine-template-add <tsuru_admin_machine_template_add_cmd>` command.

.. _tsuru_admin_docker_node_list_cmd:

docker-node-list
----------------

.. highlight:: bash

::

    $ tsuru-admin docker-node-list [-f/--filter <metadata>=<value>]

This command list all nodes present in the cluster. It will also show you metadata
associated to each node and the IaaS ID if the node was added using tsuru builtin
IaaS providers.

Using the ``-f/--filter`` flag, the user is able to filter the nodes that
appear in the list based on the key pairs displayed in the metadata column.
Users can also combine filters with multiple listings of ``-f``:

.. highlight:: bash

::

    $ tsuru-admin docker-node-list -f pool=mypool -f LastSuccess=2014-10-20T15:28:28-02:00

.. _tsuru_admin_docker_node_remove_cmd:

docker-node-remove
------------------

.. highlight:: bash

::

    $ tsuru-admin docker-node-remove <address> [--destroy]

This command removes a node from the cluster. Optionally it also destroys the
created IaaS machine if the ``--destroy`` flag is passed.

.. _tsuru_admin_platform_add_cmd:

platform-add
------------

.. highlight:: bash

::

    $ tsuru-admin platform-add <name> [--dockerfile]

This command allow you to add a new platform to your tsuru installation.
It will automatically create and build a whole new platform on tsuru server and
will allow your users to create apps based on that platform.

The --dockerfile flag is an URL to a dockerfile which will create your platform.

.. _tsuru_admin_platform_update_cmd:

platform-update
---------------

.. highlight:: bash

::

    $ tsuru-admin platform-update <name> [-d/--dockerfile]

This command allow you to update a platform in your tsuru installation.
It will automatically rebuild your platform and will flag apps to update
platform on next deploy.

The --dockerfile flag is an URL to a dockerfile which will update your platform.

.. _tsuru_admin_machines_list_cmd:

machines-list
-------------

.. highlight:: bash

::

    $ tsuru-admin machines-list

This command will list all machines created using ``docker-node-add`` and a IaaS
provider.

.. _tsuru_admin_machine_destroy_cmd:

machine-destroy
---------------

.. highlight:: bash

::

    $ tsuru-admin machines-list <machine id>

This command will destroy a IaaS machine based on its ID.

machine-template-list
---------------------

.. highlight:: bash

::

    $ tsuru-admin machine-template-list

This command will list all templates created using ``machine-template-add``.

.. _tsuru_admin_machine_template_add_cmd:

machine-template-add
--------------------

.. highlight:: bash

::

    $ tsuru-admin machine-template-add <name> <iaas> <param>=<value>...

This command creates a new machine template to be used with ``docker-node-add``
command. This template will contain a list of parameters that will be sent to the
IaaS provider.

.. _tsuru_admin_ssh_cmd:

ssh
---

.. highlight:: bash

::

    $ tsuru-admin ssh <container-id>

This command opens a SSH connection to the container, using the API server as a
proxy. The user may specify part of the ID of the container. For example:

.. highlight:: bash

::

    $ tsuru app-info -a myapp
    Application: tsuru-dashboard
    Repository: git@54.94.9.232:tsuru-dashboard.git
    Platform: python
    Teams: admin
    Address: tsuru-dashboard.54.94.9.232.xip.io
    Owner: admin@example.com
    Deploys: 1
    Units:
    +------------------------------------------------------------------+---------+
    | Unit                                                             | State   |
    +------------------------------------------------------------------+---------+
    | 39f82550514af3bbbec1fd204eba000546217a2fe6049e80eb28899db0419b2f | started |
    +------------------------------------------------------------------+---------+
    $ tsuru-admin ssh 39f8
    Welcome to Ubuntu 14.04 LTS (GNU/Linux 3.13.0-24-generic x86_64)
    ubuntu@ip-10-253-6-84:~$


docker-healing-list
-------------------

.. highlight:: bash

::

    $ tsuru-admin docker-healing-list [--node] [--container]

This command will list all healing processes started for nodes or containers.

.. _tsuru_admin_plan_create:

plan-create
-----------

::

    $ tsuru-admin plan-create <name> -c/--cpu-share cpushare [-m/--memory memory] [-s/--swap swap] [-d/--default]

This command creates a new plan for being used when creating new apps.

The ``--cpushare`` flag defines a relative amount of cpu share for units created
in apps using this plan. This value is unitless and relative, so specifying the
same value for all plans means all units will equally share processing power.

The ``--memory`` flag defines how much physical memory a unit is able to use, in
bytes.

The ``--swap`` flag defines how much virtual swap memory a unit is able to use, in
bytes.

The ``--default`` flag sets this plan as the default plan. It means this plan will
be used when creating an app without explicitly setting a plan.


plan-remove
-----------

::

    $ tsuru-admin plan-remove <name>

This command removes an existing plan, it will no longer be available for newly
created apps. However, this won't change anything for existing apps that were
created using the removed plan. They will keep using the same value amount of
resources described by the plan.
