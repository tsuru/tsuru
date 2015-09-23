.. Copyright 2015 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

Metrics
=======

Since 0.12.x **tsuru** get metrics from `Docker <https://www.docker.com/>`_ (using `docker stats <https://docs.docker.com/reference/commandline/stats/>`_) and store this data in a time series database.

Installing
----------

You will need a `Elasticsearch <https://www.elastic.co/guide/en/elasticsearch/guide/current/_installing_elasticsearch.html>`_ and a `Logstash <https://www.elastic.co/guide/en/logstash/current/getting-started-with-logstash.html#installing-logstash>`_ installed.

tsuru send data to Logstash using udp protocol and the message is formatted in json that requires a custom Logstash configuration:

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
            protocol => "http"
            host => "<ELASTICSEARCHHOST>"
            port => "<ELASTICSEARCHPORT>"
            index => ".measure-%{client}-%{+YYYY.MM.dd}"
            index_type => "%{metric}"
        }
    }

Configuring
-----------

You should use `tsuru-admin bs-env-set` to define the config values.

The available configs are:

`METRICS_INTERVAL` is the interval in seconds between metrics collecting and reporting from bs to the metric backend. The default value is 60 seconds.

`METRICS_BACKEND` is the metric backend. Supported backends are logstash and statsd.

.. note::

    In production we recommend logstash/elasticsearch

Logstash specific configs:

`METRICS_LOGSTASH_CLIENT` is the client name used to identify who is sending the metric. The default value is tsuru.

`METRICS_LOGSTASH_PORT` is the Logstash port. The default value is 1984.

`METRICS_LOGSTASH_HOST` is the Logstash host. The default value is localhost.

Statsd specific configs:

`METRICS_STATSD_PREFIX` is the prefix for the Statsd key. The key is composed by {prefix}tsuru.{appname}.{hostname}. The default value is an empty string "".

`METRICS_STATSD_PORT` is the Statsd port. The default value is 8125.

`METRICS_STATSD_HOST` is the Statsd host. The default value is localhost.

Metrics graph on tsuru-dashboard
--------------------------------

**tsuru-dashboard** can be used to show a graphic for each metric by application.

To enable it define the `METRICS_ELASTICSEARCH_HOST` using `tsuru-admin bs-env-set`.

.. note::

    tsuru-dashboard supports only logstash/elasticsearch backend.
