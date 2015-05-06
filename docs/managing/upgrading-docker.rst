.. Copyright 2015 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

Upgrading Docker
================

A :ref:`node <concepts_nodes>` is a physical or virtual machine with Docker
installed.  The nodes should contains one or more units (containers).

Sometimes will be necessary to upgrade the Docker. It is recommended that you
use the latest Docker version.

The simple way to do it is just upgrade Docker. You can do it following the
`official guide <https://docs.docker.com/installation/binaries/#upgrades>`_.

This operation can cause some period of downtime in an application.

How to upgrade Docker with no application downtime
--------------------------------------------------

.. note::

  You should use this guide to upgrade the entire host (a new version of the
  Linux distro, for instance) or Docker itself.

A way to upgrade with no downtime is to move all containers from the node that
you want to upgrade to another node, upgrade the node and then move the
containers back.

You can do it using the command `tsuru-admin containers-move
<http://tsuru-admin.readthedocs.org/en/latest/#containers-move>`_:

.. highlight:: bash

::

    $ tsuru-admin containers-move <from host> <to host>
