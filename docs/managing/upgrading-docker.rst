.. Copyright 2014 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

Upgrading Docker
================

A :ref:`node <concepts_nodes>` is a physical or virtual machine with Docker installed.
The nodes should contains one or more units (containers).

Sometimes will be necessary to upgrade the Docker. It is recommended that
you use the latest Docker version.

The simple way to do it is just upgrade Docker. You can do it following the `official guide <https://docs.docker.com/installation/binaries/#upgrades>`_.

This operation can raise a downtime in the units deployed in the nodes.

How upgrade without downtime
----------------------------

.. note::

  You should use this guide to upgrade the distro version or some packages.

A way to upgrade without downtime is to move all units from the node that you want to upgrade to
another node and upgrade the Docker after the move.

You can do it using the `tsuru-admin containers-move <http://tsuru-admin.readthedocs.org/en/latest/#containers-move>`_ command:

.. highlight:: bash

::

    $ tsuru-admin containers-move <from host> <to host>
