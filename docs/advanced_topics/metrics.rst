Metrics
=======

.. note::

    Currently tsuru supports statsd and graphite.

**tsuru** sends metrics data using statsd protocol and **tsuru-dashboard** (web interface) shows these data using graphite protocol.

Sending metrics
---------------

By default **tsuru** sends the metrics to `localhost:8125` on each unit. You can configure the statsd host and port defining the `STATSD_PORT` and `STATSD_HOST` environment variables.

.. note::

    If you don't want to have your own statsd/graphite infrastructure, you can install a client to get the data from localhost and send to a private server that supports statsd protocol.

Metrics graph on tsuru-dashboard
--------------------------------

**tsuru-dashboard** displays a graphic for each metric. To know where to get the metric data, the dashboards get the
`GRAPHITE_HOST` environment variable from the application.

Kind of metrics
---------------

* net.connections - the number of connections established
* cpu_max - cpu utilization
* mem_max - memory utilization
