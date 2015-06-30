.. Copyright 2015 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

bs
==

bs (or big-sibling) is a component tsuru component, responsible for reporting
information on application containers, these information include application
logs and unit status.

bs runs inside a dedicated container on every Docker node, and collects
information about all other containers running in the node, reporting their
status to tsuru.

It also binds to the rsyslog protocol, so Docker can send the logs of the
containers to the syslog server created in the bs container, and then bs can
multiplex container logs to other rsyslog endpoints and also to the tsuru API.

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

bs is also a rsyslog server, that listens to logs from containers and
multiplexes them among other rsyslog servers and the tsuru API.

Whenever starting an application container, tsuru will configure Docker to send
the logs of the containers to bs using the `syslog logging driver
<https://docs.docker.com/reference/run/#logging-driver-syslog>`_, having bs as
the destination daemon.

When receiving the logs, bs will forward them to the tsuru API, so users can
check their logs using the command ``tsuru app-log``. It can also forward the
logs to other syslog servers, using the ``docker:bs:syslog-forward-addresses``
config entry. For more detail, check the :ref:`bs configuration reference
<config_bs>`.
