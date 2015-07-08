.. Copyright 2015 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

bs
==

bs (or big sibling) is a component tsuru component, responsible for reporting
information on application containers, this information include application
logs, metrics and unit status.

bs runs inside a dedicated container on each Docker node and collects
information about all its sibling containers running on the same Docker node.

It also creates a syslog server, which is responsible for receiving all logs
from sibling containers. bs will then send these log entries to tsuru API and is
also capable of forwarding log entries to multiple remote syslog endpoints.

The sections below describe in details all the features of bs. The
:ref:`configuration <config_bs>` reference contains more information on
settings that control the way bs behaves.

Status Reporting
++++++++++++++++

bs communicates with the Docker API to collect information about containers,
and report them to the tsuru API. The bs "component" responsible for that is
the status reporter (or simply reporter).

The status reporter can connect to the Docker API through TCP or Unix Socket.
It's recommended to use Unix socket, so application containers can't talk to
the Docker API. In order to do that, the ``docker:bs:socket`` configuration
entry must be defined to the path of Docker socket in the Docker node. If this
setting is not defined, bs will use the TCP endpoint.

After collecting the data in the Docker API, the reporter will send it to the
tsuru API, and may take a last action before exiting: it can detect and kill
zombie containers, i.e. application containers that are running, but are not
known by tsuru. It doesn't mess with any container not managed by tsuru.

Logging
+++++++

bs is also a syslog server, that listens to logs from containers and multiplexes
them among other syslog servers and the tsuru API.

Whenever starting an application container, tsuru will configure Docker to send
the logs of the containers to bs using the `syslog logging driver
<https://docs.docker.com/reference/run/#logging-driver-syslog>`_, having bs as
the destination daemon.

When receiving the logs, bs will forward them to the tsuru API, so users can
check their logs using the command ``tsuru app-log``. It can also forward the
logs to other syslog servers, using the ``docker:bs:syslog-forward-addresses``
config entry. For more detail, check the :ref:`bs configuration reference
<config_bs>`.

Environment Variables
+++++++++++++++++++++

It's possible to set environment variables in started bs containers. This can be
done using the ``tsuru-admin bs-env-set`` command.

Some variables can be used to configure how the default bs application will
behave. A custom bs image can also make use of set variables to change their
behavior.

STATUS_INTERVAL
---------------

``STATUS_INTERVAL`` is the interval in seconds between status collecting and
reporting from bs to the tsuru API. The default value is 60 seconds.

SYSLOG_FORWARD_ADDRESSES
------------------------

``SYSLOG_FORWARD_ADDRESSES`` is a comma separated list of SysLog endpoints to
which bs will forward the logs from Docker containers. Log entries will be
rewritten to properly identify the application and process responsible for the
entry. The default value is an empty string, which means that bs will not
forward logs to any syslog server, only to tsuru API.
