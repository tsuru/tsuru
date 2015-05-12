.. Copyright 2015 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

+++++++
Logging
+++++++

tsuru aggregates stdout and stderr from every application process making it easier
to troubleshoot problems. To use the log make sure that your application is
sending the log to stdout and stderr.

Watch your logs
===============

To see the logs for your application. You can use the `tsuru app-log` command:

.. highlight:: bash

::

    $ tsuru app-log -a <appname>
    2014-12-11 16:36:17 -0200 [tsuru][api]:  ---> Removed route from unit 1d913e0910
    2014-12-11 16:36:17 -0200 [tsuru][api]: ---- Removing 1 old unit ----
    2014-12-11 16:36:22 -0200 [app][11f863b2c14b]: Starting gunicorn 18.0
    2014-12-11 16:36:22 -0200 [app][11f863b2c14b]: Listening at: http://0.0.0.0:8100 (51)
    2014-12-11 16:36:22 -0200 [app][11f863b2c14b]: Using worker: sync
    2014-12-11 16:36:22 -0200 [app][11f863b2c14b]: Booting worker with pid: 60
    2014-12-11 16:36:28 -0200 [tsuru][api]:  ---> Removed old unit 1/1

By default is showed the last ten log lines. If you want see more lines,
you can use the `-l/--lines` parameter:

.. highlight:: bash

::

    $ tsuru app-log -a <appname> --lines 100

Filtering
---------

You can filter logs by unit and by source.

To filter by unit you should use `-u/--unit` parameter:

.. highlight:: bash

::

    $ tsuru app-log -a <appname> --unit 11f863b2c14b
    2014-12-11 16:36:22 -0200 [app][11f863b2c14b]: Starting gunicorn 18.0
    2014-12-11 16:36:22 -0200 [app][11f863b2c14b]: Listening at: http://0.0.0.0:8100 (51)
    2014-12-11 16:36:22 -0200 [app][11f863b2c14b]: Using worker: sync

.. seealso::

    To get the unit id you can use the `tsuru app-info -a <appname>` command.

The log can be sent by your process or by tsuru api. To filter by source
you should use `-s/--source` parameter:

.. highlight:: bash

::

    $ tsuru app-log -a <appname> --source app
    2014-12-11 16:36:22 -0200 [app][11f863b2c14b]: Starting gunicorn 18.0
    2014-12-11 16:36:22 -0200 [app][11f863b2c14b]: Listening at: http://0.0.0.0:8100 (51)
    2014-12-11 16:36:22 -0200 [app][11f863b2c14b]: Using worker: sync

    $ tsuru app-log -a <appname> --source tsuru
    2014-12-11 16:36:17 -0200 [tsuru][api]:  ---> Removed route from unit 1d913e0910
    2014-12-11 16:36:17 -0200 [tsuru][api]: ---- Removing 1 old unit ----

Realtime logging
----------------

`tsuru app-log` has a `-f/--follow` option that causes the log to not stop and wait for the
new log data. With this option you can see in real time the behaviour of your application that
is useful to debug problems:

.. highlight:: bash

::

    $ tsuru app-log -a <appname> --follow

You can close the session pressing Ctrl-C.

Limitations
-----------

The tsuru native log system is designed to be fast and show the recent
log of your application. The tsuru log doesn't store all log entries for your application.

If you want to store and see all log entries you should use an external log aggregator.

Using an external log aggregator
================================

You can also send the log to an external log aggregator. To do this, tsuru uses
the `Syslog <https://tools.ietf.org/html/rfc5424>`_ protocol.

To use Syslog you should set the following environment variables in your
application:

.. highlight:: bash

::

    TSURU_SYSLOG_SERVER
    TSURU_SYSLOG_PORT (probably 514)
    TSURU_SYSLOG_FACILITY (something like local0)
    TSURU_SYSLOG_SOCKET (tcp or udp)

You can use the command `tsuru env-set` to set these enviroment variables in
your application:

.. highlight:: bash

::

    $ tsuru env-set -a myapp TSURU_SYSLOG_SERVER=myserver.com TSURU_SYSLOG_PORT=514 TSURU_SYSLOG_FACILITY=local0 TSURU_SYSLOG_SOCKET=tcp
