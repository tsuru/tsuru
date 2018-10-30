.. Copyright 2016 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

+++++++++++++++++++++++++
Managing Application Logs
+++++++++++++++++++++++++

Applications running on tsuru should send all their log messages to stdout and
stderr. This will allow docker to capture these logs and forward them according
to instructions configured by tsuru.

There are basically two ways to setup application logs in tsuru. Through bs
container and directly to an external log service. The sections below will talk
about the configuration options and advantages of each setup.

bs
==

bs (or big sibling) is a container started automatically by tsuru on every
docker node created or registered in tsuru. It's responsible for reporting
information on application containers, this information include metrics, unit
status and can also include container logs.

On a default tsuru installation all container started on docker will be
configured to send logs to the bs container using the syslog protocol. The bs
container will then send the logs to the tsuru api server and to any number of
configured external syslog servers. Similar to the diagram below:

.. diagram created using http://stable.ascii-flow.appspot.com/

::

   Docker Node
   +---------------------------------------------------------+       +---------------------+
   |                                               syslog    |       |                     |
   |                                              +----------------->| ext syslog server 1 |
   |  +-----------------+ syslog                  |(optional)|       |                     |
   |  |  app container  |+----------+             |          |       +---------------------+
   |  +-----------------+           |             +          |
   |                                |      +--------------+  |       +---------------------+
   |                                +----->|              |syslog    |                     |
   |                                       | bs container |+-------->| ext syslog server 2 |
   |                                +----->|              |(optional)|                     |
   |                                |      +--------------+  |       +---------------------+
   |  +-----------------+ syslog    |             +          |
   |  |  app container  |+----------+             |          |
   |  +-----------------+                         |          |
   |                                              |          |
   |                                              |          |
   |                                              |          |
   +----------------------------------------------|----------+
                                                  |
                                                  |
   +-------------------+                          |
   |                   |  websocket (optional)    |
   | tsuru api server  |<-------------------------+
   |                   |
   +-------------------+


For informations about how to configure bs to forward logs and also some tunning
options, please refer to the `bs documentation
<https://github.com/tsuru/bs#environment-variables>`_.

The advantage of having the bs container as an intermediary is that it knows how
to talk to the tsuru api server. Sending logs to the tsuru api server enables
the ``tsuru app-log`` command which can be used to quickly troubleshoot problems
with the application without the need of a third-party tool to read the logs.

However, tsuru api server is NOT a permanent log storage, only the latest 5000
log lines from each application are stored. If a permanent storage is required
an external syslog server must be configured.

Direct
======

tsuru can be configured to completely bypass bs when sending logs. This can be
done using the ``tsuru docker-log-update`` command. See the command
`reference documentation <https://tsuru-client.readthedocs.org/en/latest/reference.html#application-logging>`_
for more details.

When a ``log-driver`` different from ``bs`` is chosen, the logs will be similar
to the diagram below:

::

   Docker Node
   +-----------------------+
   |                       |
   |  +-----------------+  |
   |  |  app container  |-+|
   |  +-----------------+ |chosen driver     +---------------------+
   |                      +----------------->|                     |
   |                       |                 | external log server |
   |                      +----------------->|                     |
   |  +-----------------+ |chosen driver     +---------------------+
   |  |  app container  |-+|
   |  +-----------------+  |
   |                       |
   +-----------------------+

The downside of using a direct logs is that the tsuru api server will NOT
receive any log messages anymore. As a consequence the command ``tsuru app-log``
will be disabled and users will have to refer to the chosen log driver to read
log messages.
