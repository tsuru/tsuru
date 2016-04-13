.. Copyright 2016 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

Metrics
=======

Since 0.12.x **tsuru** is capable of reading metrics from `Docker
<https://www.docker.com/>`_ (using `docker stats
<https://docs.docker.com/reference/commandline/stats/>`_) and store this data in
a time series database.

Installing
----------

You will need a `Elasticsearch <https://www.elastic.co/guide/en/elasticsearch/reference/current/_installation.html>`_ and a `Logstash <https://www.elastic.co/guide/en/logstash/current/getting-started-with-logstash.html#installing-logstash>`_ installed.

tsuru send data to Logstash using udp protocol and the message is formatted in
json that requires a custom Logstash configuration:

.. highlight:: ruby

::

    input {
        udp {
            port => 1984
        }
    }

    filter {
        json {
            source => "message"
        }

        if "_jsonparsefailure" in [tags] {
            mutate {
                add_field => {
                    client => "error"
                    metric => "metric_error"
                }
            }
        }
    }

    output {
        elasticsearch {
            hosts => ["http://ELASTICSEARCHHOST:ELASTICSEARCHPORT"]
            index => ".measure-%{client}-%{+YYYY.MM.dd}"
            document_type => "%{metric}"
        }
    }

Configuring
-----------

You should use `tsuru-admin node-container-update big-sibling --env NAME=VALUE`
to define the config values.

The available configs are:

`METRICS_INTERVAL` is the interval in seconds between metrics collecting and
reporting from bs to the metric backend. The default value is 60 seconds.

`METRICS_BACKEND` is the metric backend. Only 'logstash' is supported right now.

Logstash specific configs:

`METRICS_LOGSTASH_CLIENT` is the client name used to identify who is sending the
metric. The default value is tsuru.

`METRICS_LOGSTASH_PORT` is the Logstash port. The default value is 1984.

`METRICS_LOGSTASH_HOST` is the Logstash host. The default value is localhost.

Metrics graph on tsuru-dashboard
--------------------------------

**tsuru-dashboard** can be used to show a graphic for each metric by
application.

To enable it define the `METRICS_ELASTICSEARCH_HOST` using `tsuru-admin
node-container-update big-sibling --env`.
