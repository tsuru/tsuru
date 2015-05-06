.. Copyright 2015 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

+++++++++++++++++++++++++
Recovering an application
+++++++++++++++++++++++++

Your application may be down for a number of reasons. This page can help you
discover why and guide you to fix the problem.

Check your application logs
===========================

tsuru aggregates stdout and stderr from every application process making easier
to troubleshoot problems.

To know more how the tsuru log works see the :doc:`log documentation
</using/logging>`.

Restart your application
========================

Some application issues are solved by a simple restart. For example, your
application may need to be restarted after a schema change to your database.

.. highlight:: bash

::

    $ tsuru app-restart -a appname

Checking the status of application units
========================================

.. highlight:: bash

::

    $ tsuru app-info -a appname

Open a shell to the application
===============================

You can also use `tsuru app-shell` to open a remote shell to one of the units
of the application.

.. highlight:: bash

::

    $ tsuru app-shell -a appname

You can also specify the unit ID to connect:

.. highlight:: bash

::

    $ tsuru app-shell -a appname <container-id>
