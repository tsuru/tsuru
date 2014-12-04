.. Copyright 2014 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

+++++++++++++++++++++++++
Recovering an application
+++++++++++++++++++++++++

Your application may be down for a number of reasons. 
This page can help you discover why and guide you to fix the problem.

Check your application logs
===========================

The first step is to check the application logs. To view your logs, run:

.. highlight:: bash

::

    $ tsuru log -a appname

Restart your application
========================

Some application issues are solved by restart. 
For example, your application may need to be restarted after a 
schema change to your database.

.. highlight:: bash

::

    $ tsuru restart -a appname

Checking units status
=====================

.. highlight:: bash

::

    $ tsuru app-info -a appname
