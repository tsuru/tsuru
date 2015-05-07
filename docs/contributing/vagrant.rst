.. Copyright 2015 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

+++++++++++++++++++++++++++++++++++++++++++++++
Building a development environment with Vagrant
+++++++++++++++++++++++++++++++++++++++++++++++

First, make sure that one of the supported Vagrant providers, Vagrant_, and
Git_ are installed on your machine.

Then clone the tsuru-bootstrap_ project from GitHub:

::

    $ git clone https://github.com/tsuru/tsuru-bootstrap.git

Enter the ``tsuru-bootstrap`` directory and execute ``vagrant up``, defining
the environment variable TSURU_NOW_OPTIONS as "--tsuru-from-source". It will
take some time:

::

    $ cd tsuru-bootstrap
    $ TSURU_NOW_OPTIONS="--tsuru-from-source" vagrant up

You can optionally specify a provider with the ``--provider`` parameter. The
following providers are configured in the Vagrantfile:

* VirtualBox
* EC2
* Parallels Desktop

Then configure the tsuru target with the address of the server that vagrant is using:

::

    $ tsuru target-add development http://192.168.50.4:8080 -s

Now you can create your user and deploy your apps.


.. _VirtualBox: https://www.virtualbox.org/
.. _Vagrant: http://vagrantup.com/
.. _git: http://git-scm.com/
.. _tsuru-bootstrap: https://github.com/tsuru/tsuru-bootstrap
