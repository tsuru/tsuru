.. Copyright 2014 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

+++++++++++++++++
tsuru-admin usage
+++++++++++++++++

tsuru-admin command supports administrative operations on a tsuru server.
It can be compiled with:

.. highlight:: bash

::

    $ go get github.com/tsuru/tsuru/cmd/tsuru-admin


Setting a target
================

The target for the tsuru-admin command should point to the admin-listen
address configured in your tsuru.conf file.

.. highlight:: yaml

::

    admin-listen: "0.0.0.0:8888"


.. highlight:: bash

::

    $ tsuru-admin target-add default tsuru.myhost.com:8888
    $ tsuru-admin target-set default

Commands
========

All the "container*"" commands below only exist when using the docker
provisioner.

containers-move
---------------

.. highlight:: bash

::

    $ tsuru-admin containers-move <from host> <to host>

It allows you to move all containers from one host to another. This is useful
when doing maintenance on hosts. <from host> and <to host> must be host names
of existing docker servers. They can either be added to the docker:servers
entry in the tsuru.conf file or added dynamically if using other schedulers,
see `docker schedulers <../../provisioners/docker/schedulers.html#adding-a-node>`_ 
for more details.

This command will go through the following steps:

* Enumerate all units at the origin host;
* For each unit, create a new unit at the destination host;
* Erase each unit from the origin host.

container-move
--------------

.. highlight:: bash

::

    $ tsuru-admin container-move <container id> <to host>

This command allow you to specify a container id and a destination host, this
will create a new container on the destination host and remove the container
from its previous host.


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

platform-add
------------

.. highlight:: bash

::

    $ tsuru-admin platform-add <name> [--dockerfile]

This command allow you to add a new platform to your tsuru installation.
It will automatically create and build a whole new platform on tsuru server and
will allow your users to create apps based on that platform.

The --dockerfile flag is an URL to a dockerfile which will create your platform.
