.. Copyright 2018 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

++++++++++++++
Event webhooks
++++++++++++++

Event webhooks allow integrating tsuru events with external systems. You can create an event
webhook to notify the occurence of specific event types. When you create an event webhook,
tsuru makes a request to the specified URL for every event according specific filters.

For more info on the client commands for handling webhooks, check
`tsuru client docs <https://tsuru-client.readthedocs.io/en/master/reference.html#event-webhooks>`_.


Configurations
==============

Event webhook configurations basically involve filtering events and configuring the hook request.

Event filtering configurations
------------------------------

By default, all events that the webhook creator has access will trigger it. But the events may be
filtered by some criteria:

- Error only: triggers only failing events
- Success only: triggers only successful events
- Kind type: ``permission`` or ``internal``
- Kind name: one of the values returned by the ``tsuru permission-list`` command, like ``app.create`` or ``pool.update``
- Target type: ``global``, ``app``, ``node``, ``container``, ``pool``, ``service``, ``service-instance``, ``team``, ``user``, ``iaas``, ``role``, ``platform``, ``plan``, ``node-container``, ``install-host``, ``event-block``, ``cluster``, ``volume`` or ``webhook``
- Target value: the value according to the target type. When target type is ``app``, for instance, target value will be the app name

Hook request configurations
---------------------------

- URL: the URL of the request
- Method: ``GET``, ``POST``, ``PUT``, ``PATCH`` or ``DELETE``. Defaults to ``POST``
- Headers: HTTP headers, defined in ``key=value`` format
- Body: Payload of the request, used when the method is ``POST``, ``PUT`` or ``PATCH``. Defaults to the serialized event in JSON
- Proxy: Proxy server used for the requests

The request body may be specified with `Go templates <https://golang.org/pkg/text/template/>`_,
to use event fields as variables. Refer to `event data
<https://github.com/tsuru/tsuru/blob/a631ecea624e94875fb35ab25990ebe51b1ebccb/event/event.go#L190-L211>`_
for the available fields.


Examples
========

Notifying every successful app deploy to a `Slack <https://slack.com/>`_ channel:

.. tabs::

   .. tab:: Tsuru client

      .. highlight:: bash

      ::

          $ tsuru event-webhook-create my-webhook https://hooks.slack.com/services/...
                  -d "all successful deploys"
                  -t <my-team>
                  -m POST
                  -H Content-Type=application/x-www-form-urlencoded
                  -b 'payload={"text": "[{{.Kind.Name}}]: {{.Target.Type}} {{.Target.Value}}"}'
                  --success-only
                  --kind-name app.deploy

   .. tab:: Terraform

      .. highlight:: text

      ::

          resource "tsuru_webhook" "my-webhook" {
             name        = "my-webhook"
             description = "all sucessful deploys"
             url         = "https://hooks.slack.com/services/..."
             team_owner  = "myteam"
             method      = "POST"
             headers     = {
                "Content-Type" = "application/x-www-form-urlencoded"
             }
             body        = <<EOT
             payload={"text": "[{{.Kind.Name}}]: {{.Target.Type}} {{.Target.Value}}"}
             EOT

             event_filter {
                success_only = true
                kind_names = [
                   "app.deploy"
                ]
             }
          }


Calling a specific URL every time a specific app is updated:

.. tabs::

   .. tab:: Tsuru client

      .. highlight:: bash

      ::

          $ tsuru event-webhook-create my-webhook <my-url>
                  -t <my-team>
                  --kind-name app.update
                  --target-value <my-app>
