.. Copyright 2014 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

+++++++++++++++++++++++++++++++++++++++++++++++
Building a development environment with Vagrant
+++++++++++++++++++++++++++++++++++++++++++++++

First, make sure that VirtualBox_, Vagrant_, and git_ are installed on your machine.

Then clone the tsuru-bootstrap_ project from GitHub:

.. highlight:: bash

::

    git clone https://github.com/tsuru/tsuru-bootstrap.git

Enter the ``tsuru-bootstrap`` directory and execute ``vagrant up``. It will take some time:

.. highlight:: bash

::

    cd tsuru-bootstrap
    vagrant up

Then configure the tsuru target with the address of the server that vagrant is using:

.. highlight:: bash

::

    tsuru target-add development http://192.168.50.4:8080 -s

Now you can create your user and deploy your apps.


.. _VirtualBox: https://www.virtualbox.org/
.. _Vagrant: http://vagrantup.com/
.. _git: http://git-scm.com/
.. _tsuru-bootstrap: https://github.com/tsuru/tsuru-bootstrap
