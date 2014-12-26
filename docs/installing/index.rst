.. Copyright 2014 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

:title: Installing tsuru
:description: Step-by-step guide about how to setup a private PaaS with tsuru. 

.. _installing:

Installing
==========

If you want to try tsuru with a minimum amount of effort, we recommend you to use `tsuru
Now <https://github.com/tsuru/now>`_ (or `tsuru-bootstrap
<https://github.com/tsuru/tsuru-bootstrap>`_, which runs tsuru Now in a Vagrant VM).

tsuru Now will install tsuru API, tsuru Client, tsuru Admin, and all of their
dependencies on a single machine. It will also include a docker node which will run
deployed applications.

This gives you a very nice environment for trying out tsuru, but this is not the
recommended approach for a production environment. This
document will describe how to install each component separately.

We assume that tsuru is being installed on an Ubuntu Server 14.04 LTS 64-bit
machine. This is currently the supported environment for tsuru, you may try
running it on other environments, but there's a chance it won't be a smooth ride.

.. toctree::

    gandalf
    api
    hipache-router
    adding-nodes
