.. Copyright 2012 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

:title: Installing tsuru
:description: Step-by-step guide about how to setup a private PaaS with tsuru.

.. _installing:

Installing
==========

.. note::
    tsuru client ships with a feature called :ref:`using-tsuru-installer`
    that can be used to provision machines on different IaaSs and installs tsuru
    on them.

    This gives you a very nice environment for trying out tsuru, but this is not
    the recommended approach for a production environment.


.. note::

    Other methods of installation like `tsuru Now <https://github.com/tsuru/now>`_
    and `tsuru-bootstrap <https://github.com/tsuru/tsuru-bootstrap>`_ are deprecated.


This document will describe how to install each component separately.
We assume that tsuru is being installed on an Ubuntu Server 14.04 LTS 64-bit
machine. This is currently the supported environment for tsuru, you may try
running it on other environments, but there's a chance it won't be a smooth
ride.

.. toctree::

    api
    planb-router
    adding-nodes
    dashboard
    Gandalf (Optional) <gandalf>
