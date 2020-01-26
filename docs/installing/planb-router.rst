.. Copyright 2015 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

++++++++++++
PlanB Router
++++++++++++

`PlanB <https://github.com/tsuru/planb/>`_ is a distributed HTTP and
websocket proxy. It's built on top of a configuration pattern defined by
`Hipache <https://github.com/hipache/hipache/>`_.

tsuru uses PlanB to route the requests to the containers. Routing information is
stored by tsuru in the configured Redis server, PlanB will read this
configuration directly from Redis.

Adding repositories
===================

Let's start adding the repositories for tsuru which contain the PlanB package.

deb:

.. highlight:: bash

::

    $ curl -s https://packagecloud.io/install/repositories/tsuru/stable/script.deb.sh | sudo bash

rpm:

.. highlight:: bash

::

    $ curl -s https://packagecloud.io/install/repositories/tsuru/stable/script.rpm.sh | sudo bash

For more details, check `packagecloud.io documentation
<https://packagecloud.io/tsuru/stable/install#bash>`_.


Installing
==========

deb:

.. highlight:: bash

::

    $ sudo apt-get install planb

rpm:

.. highlight:: bash

::

    $ sudo yum install planb


Configuring
===========

You may change the file ``/etc/default/planb``, changing the PLANB_OPTS
environment variable for configuring the binding address and the Redis
endpoint, along with other settings, as `described in PlanB docs
<https://github.com/tsuru/planb#start-up-flags>`_.

Keep in mind that you will need to change the init file for service in
``/etc/systemd/system/planb.service`` and add in the line where the 
``ExecStart`` is located the loading of the environment variable.
The line will have the following content: 
``ExecStart=/usr/bin/planb $PLANB_OPTS``.

After changing the file, you only need to start PlanB with:

.. highlight:: bash

::

    sudo start planb
