.. Copyright 2014 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

+++++++
Logging
+++++++

tsuru aggregates stdout and stderr from every application process making it
easier to troubleshoot problems. To use the log make sure that your application
is sending the log to stdout and stderr.

Watch your logs
===============

On its default installation tsuru will have all logs available using the ``tsuru
app log`` command.

It's possible that viewing logs using tsuru was disabled by an administrator. In
this case running ``tsuru app log`` will show instructions on how logs can be
read.


Basic usage
-----------

.. highlight:: bash

::

    $ tsuru app log -a <appname>
    2014-12-11 16:36:17 -0200 [tsuru][api]:  ---> Removed route from unit 1d913e0910
    2014-12-11 16:36:17 -0200 [tsuru][api]: ---- Removing 1 old unit ----
    2014-12-11 16:36:22 -0200 [app][11f863b2c14b]: Starting gunicorn 18.0
    2014-12-11 16:36:22 -0200 [app][11f863b2c14b]: Listening at: http://0.0.0.0:8100 (51)
    2014-12-11 16:36:22 -0200 [app][11f863b2c14b]: Using worker: sync
    2014-12-11 16:36:22 -0200 [app][11f863b2c14b]: Booting worker with pid: 60
    2014-12-11 16:36:28 -0200 [tsuru][api]:  ---> Removed old unit 1/1

By default is showed the last ten log lines. If you want see more lines,
you can use the ``-l/--lines`` parameter:

.. highlight:: bash

::

    $ tsuru app log -a <appname> --lines 100

Filtering
---------

You can filter logs by unit and by source.

To filter by unit you should use `-u/--unit` parameter:

.. highlight:: bash

::

    $ tsuru app log -a <appname> --unit 11f863b2c14b
    2014-12-11 16:36:22 -0200 [app][11f863b2c14b]: Starting gunicorn 18.0
    2014-12-11 16:36:22 -0200 [app][11f863b2c14b]: Listening at: http://0.0.0.0:8100 (51)
    2014-12-11 16:36:22 -0200 [app][11f863b2c14b]: Using worker: sync

.. seealso::

    To get the unit id you can use the ``tsuru app info -a <appname>`` command.

The log can be sent by your process or by tsuru api. To filter by source
you should use ``-s/--source`` parameter:

.. highlight:: bash

::

    $ tsuru app log -a <appname> --source app
    2014-12-11 16:36:22 -0200 [app][11f863b2c14b]: Starting gunicorn 18.0
    2014-12-11 16:36:22 -0200 [app][11f863b2c14b]: Listening at: http://0.0.0.0:8100 (51)
    2014-12-11 16:36:22 -0200 [app][11f863b2c14b]: Using worker: sync

    $ tsuru app log -a <appname> --source tsuru
    2014-12-11 16:36:17 -0200 [tsuru][api]:  ---> Removed route from unit 1d913e0910
    2014-12-11 16:36:17 -0200 [tsuru][api]: ---- Removing 1 old unit ----

Realtime logging
----------------

``tsuru app log`` has a ``-f/--follow`` option that causes the log to not stop and
wait for the new log data. With this option you can see in real time the
behaviour of your application that is useful to debug problems:

.. highlight:: bash

::

    $ tsuru app log -a <appname> --follow

You can close the session pressing Ctrl-C.
