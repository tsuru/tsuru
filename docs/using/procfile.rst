.. Copyright 2015 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

++++++++
Procfile
++++++++

`Procfile` is a simple text file called `Procfile` that describe the components required
to run an applications. It is the way to tell to `tsuru` how to run
your applications.

This document describes some of the more
advances features of and the Procfile ecosystem.

A `Procfile` should look like:

.. highlight:: bash

::

    web: gunicorn -w 3 wsgi

Syntax
======

`Procfile` is a plain text file called `Procfile` placed at the root of your
application.

Each project should be represented by a name and a command,
like below:

.. highlight:: bash

::

    <name>: <command>

The `name` is a string which may contain alphanumerics
and underscores and identifies one type of process.

`command` is a shell commandline which will be executed to
spawn a process.

Environment variables
=====================

You can reference yours environment variables in the command:

.. highlight:: bash

::

    web: ./manage.py runserver 0.0.0.0:$PORT

For more information about `Procfile` you can see the honcho documentation
about `Procfiles`: http://honcho.rtfd.org/en/latest/using_procfiles.html.
