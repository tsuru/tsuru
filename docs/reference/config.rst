.. Copyright 2013 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

++++++++++++++++++++
tsuru.conf reference
++++++++++++++++++++

tsuru uses a configuration file in `YAML <http://www.yaml.org/>`_ format. This
document describes what each option means, and how it should look.

Notation
========

tsuru uses a colon to represent nesting in YAML. So, whenever this document says
something like ``key1:key2``, it refers to the value of the ``key2`` that is
nested in the block that is the value of ``key1``. For example,
``database:url`` means:

.. highlight:: yaml

::

    database:
      url: <value>

tsuru configuration
===================

This section describes tsuru's core configuration. Other sections will include
configuration of optional components, and finally, a full sample file.

HTTP server
-----------

tsuru provides a REST API, that supports HTTP and HTTP/TLS (a.k.a. HTTPS). Here
are the options that affect how tsuru's API behaves:

listen
++++++

``listen`` defines in which address tsuru webserver will listen. It has the
form <host>:<port>. You may omit the host (example: ``:8080``). This setting
has no default value.

shutdown-timeout
++++++++++++++++

``shutdown-timeout`` defines how many seconds to wait when performing an api
shutdown (by sending SIGTERM or SIGQUIT). Defaults to 600 seconds.

use-tls
+++++++

``use-tls`` indicates whether tsuru should use TLS or not. This setting is
optional, and defaults to "false".

tls:listen
++++++++++

If both this and ``listen`` keys are set (following the same rules as ``listen``
key), tsuru will start two webserver instances: one with HTTP on ``listen``
address, and the other one with HTTPS on ``tls:listen`` address.
If only one of ``listen`` and ``tls:listen`` keys is set (and ``use-tls`` is
true), tsuru will only run the TLS supporting webserver. This setting is
optional, unless ``use-tls`` is true.

tls:cert-file
+++++++++++++

``tls:cert-file`` is the path to the X.509 certificate file configured to serve
the domain.  This setting is optional, unless ``use-tls`` is true.

tls:key-file
++++++++++++

``tls:key-file`` is the path to private key file configured to serve the
domain. This setting is optional, unless ``use-tls`` is true.

tls:validate-certificate
++++++++++++++++++++++++

``tls:validate-certificate`` prevents an invalid certificate from being offered
to web clients or not. This setting is optional and defaults to "false".

If enabled, the server will validate the certificates before server's start and
during the certificates auto-reload process (if any). If a certificate expires
during server execution (after loaded by server) this feature will gracefully
shutdown the server.

tls:auto-reload:interval
++++++++++++++++++++++++

``tls:auto-reload:interval`` defines the time frequency which the TLS
certificates are reloaded by the webserver. A new certificate only is loaded
when there is difference between newer and older.

This setting is optional. The default value is 0, which means automatic reload
is not enabled. To enable it, you could use any `time.Duration <https://golang.org/pkg/time/#Duration>`_
value greater than zero, as well as `parseable values <https://golang.org/pkg/time/#ParseDuration>`_
as: "1h", "60m", "3600s", etc.

server:read-timeout
+++++++++++++++++++

``server:read-timeout`` is the timeout of reading requests in the server. This
is the maximum duration of any request to the tsuru server.

This is useful to avoid leaking connections, in case clients drop the
connection before end sending the request. The default value is 0, meaning no
timeout.

server:write-timeout
++++++++++++++++++++

``server:write-timeout`` is the timeout of writing responses in the server.

This is useful to avoid leaking connections, in case clients drop the
connection before reading the response from tsuru. The default value is 0,
meaning no timeout.

server:app-log-buffer-size
++++++++++++++++++++++++++

The maximum number of received log messages from applications to hold in memory
waiting to be sent to the log database. The default value is 500000.


disable-index-page
++++++++++++++++++

tsuru API serves an index page with some basic instructions on how to use the
current target. It's possible to disable this page by setting the
``disable-index-page`` flag to true. It's also possible to customize which
template will be used in the index page, see the next configuration entry for
more details.

This setting is optional, and defaults to ``false``.

index-page-template
+++++++++++++++++++

``index-page-template`` is the template that will be used for the index page.
It must use the `Go template syntax <http://golang.org/pkg/text/template/>`_,
and tsuru will provide the following variables in the context of the template:

    - ``tsuruTarget``: the target URL of the tsuru API serving the index page
    - ``userCreate``: a boolean indicating whether user registration is enabled
      or disabled
    - ``nativeLogin``: a boolean indicating whether the API is configured to
      use the native authentication scheme
    - ``keysEnabled``: a boolean indicating whether the API is configured to
      manage SSH keys

It will also include a function used for querying configuration values, named
``getConfig``. Here is an example of the function usage:

.. highlight:: html

::

    <body>
        {{if getConfig "use-tls"}}
        <p>we're safe</p>
        {{else}}
        <p>we're not safe</p>
        {{end}}
    </body>

This setting is optional. When ``index-page-template`` is not defined, tsuru
will use the `default template
<https://github.com/tsuru/tsuru/blob/main/api/index_templates.go>`__.

reset-password-template
+++++++++++++++++++++++

``reset-password-template`` is the template that will be used to "password reset" email.
It must use the `Go template syntax <http://golang.org/pkg/text/template/>`_,
and tsuru will provide the following variables in the context of the template:

    - ``Token``: a string, the id of password reset request
    - ``UserEmail``: a string, the user email
    - ``Creation``: a time, when password reset was requested
    - ``Used``: a boolean, reset-password was done or not

This setting is optional. When ``reset-password-template`` is not defined, tsuru
will use the `default template <https://github.com/tsuru/tsuru/blob/main/auth/native/data.go>`__.

reset-password-successfully-template
++++++++++++++++++++++++++++++++++++

``reset-password-successfully-template`` is the template that will be used to email with new password, after reset.
It must use the `Go template syntax <http://golang.org/pkg/text/template/>`_,
and tsuru will provide the following variables in the context of the template:

    - ``password``: a string, the new password
    - ``email``: a string, the user email

This setting is optional. When ``reset-password-template`` is not defined, tsuru
will use the `default template <https://github.com/tsuru/tsuru/blob/main/auth/native/data.go>`__.

Database access
---------------

tsuru uses MongoDB as a database manager to store information like users,
machines, containers, etc. You need to describe how tsuru will connect to your
database server. Therefore, it's necessary to provide a `MongoDB connection
string <https://docs.mongodb.org/manual/reference/connection-string/>`_.
Database related options are listed below:

database:url
++++++++++++

``database:url`` is the database connection string. It is a mandatory setting
and it has no default value. Examples of strings include basic ``127.0.0.1`` and
more advanced ``mongodb://user:password@127.0.0.1:27017/database``. Please refer
to `MongoDB documentation
<http://docs.mongodb.org/manual/reference/connection-string/>`_ for more details
and examples of connection strings.

database:name
+++++++++++++

``database:name`` is the name of the database that tsuru uses. It is a
mandatory setting and has no default value. An example of value is "tsuru".

database:driver
+++++++++++++++

``database:driver`` is the name of the database driver that tsuru uses.
Currently, the only value supported is "mongodb".

.. _config_logdb:


Email configuration
-------------------

tsuru sends email to users when they request password recovery. In order to send
those emails, tsuru needs to be configured with some SMTP settings. Omitting
these settings won't break tsuru, but users will not be able to reset their
password.

smtp:server
+++++++++++

The SMTP server to connect to. It must be in the form <host>:<port>. Example:
"smtp.gmail.com:587".

smtp:user
+++++++++

The user to authenticate with the SMTP sever. Currently, tsuru requires
authenticated sessions.

smtp:password
+++++++++++++

The password for authentication within the SMTP server.


Authentication configuration
----------------------------

tsuru has support for ``native``, ``oauth`` and ``saml`` authentication schemes.

The default scheme is ``native`` and it supports the creation of users in
tsuru's internal database. It hashes passwords brcypt. Tokens are generated
during authentication and are hashed using SHA512.

The ``auth`` section also controls whether user registration is on or off. When
user registration is off, only admin users are able to create new users.

auth:scheme
+++++++++++

The authentication scheme to be used. The default value is ``native``, the other
supported value is ``oauth``.

auth:user-registration
++++++++++++++++++++++

This flag indicates whether user registration is enabled. This setting is
optional, and defaults to false.

auth:hash-cost
++++++++++++++

Required only with ``native`` chosen as ``auth:scheme``.

This number indicates how many CPU time you're willing to give to hashing
calculation. It is an absolute number, between 4 and 31, where 4 is faster and
less secure, while 31 is very secure and *very* slow.

auth:token-expire-days
++++++++++++++++++++++

Required only with ``native`` chosen as ``auth:scheme``.

Whenever a user logs in, tsuru generates a token for him/her, and the user may
store the token. ``auth:token-expire-days`` setting defines the amount of days
that the token will be valid. This setting is optional, and defaults to "7".

auth:max-simultaneous-sessions
++++++++++++++++++++++++++++++

tsuru can limit the number of simultaneous sessions per user. This setting is
optional, and defaults to "unlimited".

auth:oauth
++++++++++

Every config entry inside ``auth:oauth`` are used when the ``auth:scheme`` is
set to "oauth". Please check `rfc6749 <http://tools.ietf.org/html/rfc6749>`_ for
more details.

auth:oauth:client-id
++++++++++++++++++++

The client id provided by your OAuth server.

auth:oauth:client-secret
++++++++++++++++++++++++

The client secret provided by your OAuth server.

auth:oauth:scope
++++++++++++++++

The scope for your authentication request.

auth:oauth:auth-url
+++++++++++++++++++

The URL used in the authorization step of the OAuth flow. tsuru CLI will receive
this URL and trigger the opening a browser on this URL with the necessary
parameters.

During the authorization step, tsuru CLI will start a server locally and set the
callback to http://localhost:<port>, if ``auth:oauth:callback-port`` is set
tsuru CLI will use its value as <port>. If ``auth:oauth:callback-port`` isn't
present tsuru CLI will automatically choose an open port.

The callback URL should be registered on your OAuth server.

If the chosen server requires the callback URL to match the same host and port
as the registered one you should register "http://localhost:<chosen port>" and
set the ``auth:oauth:callback-port`` accordingly.

If the chosen server is more lenient and allows a different port to be used you
should register simply "http://localhost" and leave ``auth:oauth:callback-port``
empty.

auth:oauth:token-url
++++++++++++++++++++

The URL used in the exchange token step of the OAuth flow.

auth:oauth:info-url
+++++++++++++++++++

The URL used to fetch information about the authenticated user. tsuru expects a
json response containing a field called ``email``.

tsuru will also make call this URL on every request to the API to make sure the
token is still valid and hasn't been revoked.

auth:oauth:collection
+++++++++++++++++++++

The database collection used to store valid access tokens. Defaults to
"oauth_tokens".

auth:oauth:callback-port
++++++++++++++++++++++++

The port used in the callback URL during the authorization step. Check docs for
``auth:oauth:auth-url`` for more details.

.. _saml_configuration:

auth:saml
+++++++++

Every config entry inside ``auth:saml`` are used when the ``auth:scheme`` is
set to "saml". Please check `SAML V2.0 specification <http://saml.xml.org/saml-specifications>`_ for
more details.

auth:saml:sp-publiccert
+++++++++++++++++++++++

Service provider public certificate path.

auth:saml:sp-privatekey
+++++++++++++++++++++++

Service provider private key path.

auth:saml:idp-ssourl
++++++++++++++++++++

Identity provider url.

auth:saml:sp-display-name
+++++++++++++++++++++++++

Service provider display name. The default value is `Tsuru`.

auth:saml:sp-description
++++++++++++++++++++++++

Service provider description. The default values is `Tsuru Platform as a Service software`.

auth:saml:idp-publiccert
++++++++++++++++++++++++

Identity provider public certificate.

auth:saml:sp-entityid
+++++++++++++++++++++

Service provider entity id.

auth:saml:sp-sign-request
+++++++++++++++++++++++++

Boolean value that indicates to service provider signs the request.
The default value is `false`.

auth:saml:idp-sign-response
+++++++++++++++++++++++++++

Boolean value that indicates to identity provider signs the response.
The default value is `false`.

auth:saml:idp-deflate-encoding
++++++++++++++++++++++++++++++

Boolean value that indicates to identity provider to enable deflate encoding.
The default value is `false`.

.. _config_pubsub:

pubsub
++++++

Deprecated: These settings are obsolete and are ignored as of tsuru 1.3.0.

.. _config_admin_user:

Quota management
----------------

tsuru can, optionally, manage quotas. Currently, there are two available
quotas: apps per user and units per app.

tsuru administrators can control the default quota for new users and new apps
in the configuration file, and use ``tsuru`` command to change quotas for
users or apps. Quota management is disabled by default, to enable it, just set
the desired quota to a positive integer.

quota:units-per-app
+++++++++++++++++++

``quota:units-per-app`` is the default value for units per-app quota. All new
apps will have at most the number of units specified by this setting. This
setting is optional, and defaults to "unlimited".

quota:apps-per-user
+++++++++++++++++++

``quota:apps-per-user`` is the default value for apps per-user quota. All new
users will have at most the number of apps specified by this setting. This
setting is optional, and defaults to "unlimited".

.. _config_logging:

Logging
-------

Tsuru supports three logging flavors, that can be enabled or disabled
altogether. The default behavior of tsuru is to send all logs to syslog, but it
can also send logs to the standard error stream or a file. It's is possible to
use any combination of the three flavors at any time in tsuru configuration
(e.g.: write logs both to stderr and syslog, or a file and stderr, or to all of
the flavors simultaneously).

There's also the possibility to enable or disable debugging log, via the debug
flag.

debug
+++++

``false`` is the default value, so you won't see any
noises on logs, to turn it on set it to true, e.g.: ``debug: true``

log:file
++++++++

Use this to specify a path to a log file. If no file is specified, tsuru-server
won't write logs to any file.

log:disable-syslog
++++++++++++++++++

``log:disable-syslog`` indicates whether tsuru-server should disable the use of
syslog. ``false`` is the default value. If it's ``true``, tsuru-server won't
send any logs to syslog.

log:syslog-tag
++++++++++++++

``log:syslog-tag`` is the tag that will be attached to every log line. The
default value is "tsr".

log:use-stderr
++++++++++++++

``log:use-stderr`` indicates whether tsuru-server should write logs to standard
error stream. The default value is ``false``.

.. _config_routers:

Routers
-------

As of 0.10.0, all your router configuration should live under entries with the
format ``routers:<router name>``.



routers:<router name>:default
+++++++++++++++++++++++++++++

Boolean value that indicates if this router is to be used when an app is created
with no specific router. Defaults to false.

Depending on the type, there are some specific configuration options available.

routers:<router name>:domain (type: galeb, vulcand)
++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++

The domain of the server running your router. Applications created with
tsuru will have a address of ``http://<app-name>.<domain>``

routers:<router name>:api-url (type: galeb, vulcand, api)
+++++++++++++++++++++++++++++++++++++++++++++++++++++++++

The URL for the router manager API.

routers:<router name>:debug (type galeb, api)
+++++++++++++++++++++++++++++++++++++++++++++

Enables debug mode, logging additional information.

routers:<router name>:username (type: galeb)
++++++++++++++++++++++++++++++++++++++++++++

Galeb manager username.

routers:<router name>:password (type: galeb)
++++++++++++++++++++++++++++++++++++++++++++

Galeb manager password.

routers:<router name>:environment (type: galeb)
+++++++++++++++++++++++++++++++++++++++++++++++

Galeb manager environment used to create virtual hosts and backend pools.

routers:<router name>:farm-type (type: galeb)
+++++++++++++++++++++++++++++++++++++++++++++

Galeb manager farm type used to create virtual hosts and backend pools.

routers:<router name>:plan (type: galeb)
++++++++++++++++++++++++++++++++++++++++

Galeb manager plan used to create virtual hosts and backend pools.

routers:<router name>:project (type: galeb)
+++++++++++++++++++++++++++++++++++++++++++

Galeb manager project used to create virtual hosts, backend pools and pools.

routers:<router name>:load-balance-policy (type: galeb)
+++++++++++++++++++++++++++++++++++++++++++++++++++++++

Galeb manager load balancing policy used to create backend pools.

routers:<router name>:rule-type (type: galeb)
+++++++++++++++++++++++++++++++++++++++++++++

Galeb manager rule type used to create rules.

routers:<router name>:use-token (type: galeb)
+++++++++++++++++++++++++++++++++++++++++++++

If true, tsuru will get an authentication token by calling the /token route and
reuse it until it expires. (Defaults to false)

routers:<router name>:max-requests (type: galeb)
++++++++++++++++++++++++++++++++++++++++++++++++

Maximum number of parallel requests to the Galeb API when adding or removing
routes. (Defaults to unlimited)

routers:<router name>:headers (type: api)
+++++++++++++++++++++++++++++++++++++++++

Headers to be added to the request to the api responsible for mananing the router. Example:

.. highlight: yaml

::

      headers:
        - X-CUSTOM-HEADER: my-value

Defining the provisioner
------------------------

tsuru has extensible support for provisioners. A provisioner is a Go type that
satisfies the `provision.Provisioner` interface. By default, tsuru will use
``DockerProvisioner`` (identified by the string "docker"). Other provisioners
are available as **experiments** and may be removed in future versions:
``swarm`` and ``kubernetes``.

.. _config_provisioner:

provisioner
+++++++++++

``provisioner`` is the string the name of the **default** provisioner that will
be used by tsuru. This setting is optional and defaults to ``docker``.

Docker provisioner configuration
--------------------------------

docker:collection
+++++++++++++++++

Database collection name used to store containers information.

.. _config_port_allocator:

docker:port-allocator
+++++++++++++++++++++

Deprecated. Currently, when using Docker as provisioner, tsuru trusts it
to allocate ports. Meaning thatwhenever a container restarts, the port might
change (usually, it changes).

docker:registry
+++++++++++++++

For tsuru to work with multiple docker nodes, you will need a docker-registry.
This should be in the form of ``hostname:port``, the scheme cannot be present.

docker:registry-max-try
+++++++++++++++++++++++

Number of times tsuru will try to send a image to registry.

.. _config_registry_auth:

docker:registry-auth:username
+++++++++++++++++++++++++++++

The username used for registry authentication. This setting is optional, for
registries with authentication disabled, it can be omitted.

docker:registry-auth:password
+++++++++++++++++++++++++++++

The password used for registry authentication. This setting is optional, for
registries with authentication disabled, it can be omitted.

docker:registry-auth:email
++++++++++++++++++++++++++

The email used for registry authentication. This setting is optional, for
registries with authentication disabled, it can be omitted.

docker:repository-namespace
+++++++++++++++++++++++++++

Docker repository namespace to be used for application and platform images. Images
will be tagged in docker as <docker:repository-namespace>/<platform-name> and
<docker:repository-namespace>/<app-name>. The default value is 'tsuru'.

docker:max-layers
+++++++++++++++++

The maximum number of layers in Docker images. This number represents the
number of times that Tsuru will reuse the previous image on application
deployment. The default value is 10.

docker:gc:dry-run
+++++++++++++++++

If set to ``true``, tsuru garbage collector won't remove old and failed images from registry.

.. _config_bs:

docker:bs:image
+++++++++++++++

This setting is deprecated in favor of dynamically configuring with
``tsuru node-container-update big-sibling --image <image>``.

docker:bs:socket
++++++++++++++++

This setting is deprecated in favor of dynamically configuring with
``tsuru node-container-update big-sibling --volume <local>:<remote> --env
DOCKER_ENDPOINT=<remote>``.

docker:bs:syslog-port
+++++++++++++++++++++

``docker:bs:syslog-port`` is the port in the Docker node that will be used by
the bs container for collecting logs. The default value is 1514.

If this value is changed bs node containers must be update with ``tsuru
node-container-update big-sibling --env
SYSLOG_LISTEN_ADDRESS=udp://0.0.0.0:<port>``.

docker:max-workers
++++++++++++++++++

Maximum amount of threads to be created when starting new containers, so tsuru
doesn't start too much threads in the process of starting 1000 units, for
instance. Defaults to 0 which means unlimited.

docker:nodecontainer:max-workers
++++++++++++++++++++++++++++++++

Same as ``docker:max-workers`` but applies only to when starting new node containers.
Defaults to 0 which means unlimited.

.. _config_docker_router:

docker:deploy-cmd
+++++++++++++++++

The command that will be called in your platform when a new deploy happens. The
default value for platforms supported in tsuru's basebuilder repository is
``/var/lib/tsuru/deploy``.

docker:security-opts
++++++++++++++++++++

This setting describes a list of security options that will be passed to
containers. This setting must be a list, and has no default value. If one wants
to specify just one value, it's still needed to use the list notation:

.. highlight: yaml

::

    docker:
      ...
      security-opts:
        - apparmor:PROFILE

For more details on the available options, please refer to the Docker
documentation: <https://docs.docker.com/reference/run/#security-configuration>.

.. _config_scheduler_memory:

docker:scheduler:total-memory-metadata
++++++++++++++++++++++++++++++++++++++

This value describes which metadata key will describe the total amount of
memory, in bytes, available to a docker node.

docker:scheduler:max-used-memory
++++++++++++++++++++++++++++++++

This should be a value between 0.0 and 1.0 which describes which fraction of the
total amount of memory available to a server should be reserved for app units.

The amount of memory available is found based on the node metadata described by
``docker:scheduler:total-memory-metadata`` config setting.

If this value is set, tsuru will try to find a node with enough unreserved
memory to fit the creation of new units, based on how much memory is required by
the plan used to create the application. If no node with enough unreserved
memory is found, tsuru will ignore memory restrictions and let the scheduler
choose any node.

.. _config_cluster_storage:

docker:cluster:storage
++++++++++++++++++++++

This setting has been removed. You shouldn't define it anymore, the only storage
available for the docker cluster is now ``mongodb``.

docker:cluster:mongo-url
++++++++++++++++++++++++

Connection URL to the mongodb server used to store information about the docker
cluster.

docker:cluster:mongo-database
+++++++++++++++++++++++++++++

Database name to be used to store information about the docker cluster.

docker:run-cmd:bin
++++++++++++++++++

The command that will be called on the application image to start the
application. The default value for platforms supported in tsuru's basebuilder
repository is ``/var/lib/tsuru/start``.

docker:run-cmd:port
+++++++++++++++++++

The tcp port that will be exported by the container to the node network. The
default value expected by platforms defined in tsuru's basebuilder repository is
``8888``.

docker:user
+++++++++++

The user tsuru will use to start the container. The default value is
``ubuntu``, which is the expected value for default tsuru platforms. An empty
for this will make tsuru use the platform image user.

docker:uid
+++++++++++

The user ID tsuru will use to start the container in provisioners that do not
support ``docker:user``. The default value is ``1000``, which is the expected
value for default tsuru platforms. The value ``-1`` can be used to make tsuru
use the platform image user.

.. _config_healing:

docker:healing:heal-nodes
+++++++++++++++++++++++++

Boolean value that indicates whether tsuru should try to heal nodes that have
failed a specified number of times. Healing nodes is only available if the node
was created by tsuru itself using the IaaS configuration. Defaults to ``false``.

docker:healing:active-monitoring-interval
+++++++++++++++++++++++++++++++++++++++++

Number of seconds between calls to <server>/_ping in each one of the docker
nodes. If this value is 0 or unset tsuru will never call the ping URL. Defaults
to 0.

docker:healing:disabled-time
++++++++++++++++++++++++++++

Number of seconds tsuru disables a node after a failure. This setting is only
valid if ``heal-nodes`` is set to ``true``. Defaults to 30 seconds.

docker:healing:max-failures
+++++++++++++++++++++++++++

Number of consecutive failures a node should have before triggering a healing
operation. Only valid if ``heal-nodes`` is set to ``true``. Defaults to 5.

docker:healing:wait-new-time
++++++++++++++++++++++++++++

Number of seconds tsuru should wait for the creation of a new node during the
healing process. Only valid if ``heal-nodes`` is set to ``true``. Defaults to
300 seconds (5 minutes).

docker:healing:heal-containers-timeout
++++++++++++++++++++++++++++++++++++++

Number of seconds a container should be unresponsive before triggering the
recreation of the container. A container is deemed unresponsive if it doesn't
call the set unit status URL (/apps/{app}/units/{unit}) with a ``started``
status. If this value is 0 or unset tsuru will never try to heal unresponsive
containers. Defaults to 0.

docker:healing:events_collection
++++++++++++++++++++++++++++++++

Collection name in mongodb used to store information about triggered healing
events. Defaults to ``healing_events``.

.. _config_healthcheck_max_time:

docker:healthcheck:max-time
+++++++++++++++++++++++++++

Maximum time in seconds to wait for deployment time health check to be
successful. Defaults to 120 seconds.

.. _config_image_history_size:

docker:image-history-size
+++++++++++++++++++++++++

Number of images available for rollback using ``tsuru app deploy rollback``.
tsuru will try to delete older images, but it may not be able to due to it being
used as a layer to a newer image. tsuru will keep trying to remove these old
images until they are not used as layers anymore. Defaults to 10 images.

.. _config_docker_auto_scale:


docker:auto-scale:wait-new-time
+++++++++++++++++++++++++++++++

Number of seconds tsuru should wait for the creation of a new node during the
scaling up process. Defaults to 300 seconds (5 minutes).

docker:auto-scale:group-by-metadata
+++++++++++++++++++++++++++++++++++

Deprecated. The ``pool`` is used to group nodes.

docker:auto-scale:metadata-filter
+++++++++++++++++++++++++++++++++

The name of a pool where auto scale will be enabled. Leave unset to allow
dynamically configuring with ``tsuru node-autoscale-rule-set``.


docker:auto-scale:run-interval
++++++++++++++++++++++++++++++

Number of seconds between two periodic runs of the auto scaling algorithm.
Defaults to 3600 seconds (1 hour).

.. _docker_limit:

docker:limit:actions-per-host
+++++++++++++++++++++++++++++

The maximum number of simultaneous actions to run on a docker node. When the
number of running actions is greater then the limit further actions will block
until another action has finished. Setting this limit may help the stability of
docker nodes with limited resources. If this value is set to ``0`` the limit is
disabled. Default value is ``0``.

docker:limit:mode
+++++++++++++++++

The way tsuru will ensure ``docker:limit:actions-per-host`` limit is being
respected. Possible values are ``local`` and ``global``. Defaults to ``local``.
In ``local`` mode tsuru will only limit simultaneous actions from the current
tsurud process. ``global`` mode uses MongoDB to ensure all tsurud servers using
respects the same limit.

.. _docker_sharedfs:

docker:sharedfs
+++++++++++++++

Used to create shared volumes for apps.

docker:sharedfs:hostdir
+++++++++++++++++++++++

Directory on host machine to access shared data with installed apps.

docker:sharedfs:mountpoint
++++++++++++++++++++++++++

Directory inside the container that point to ``hostdir`` directory configured
above.

docker:sharedfs:app-isolation
+++++++++++++++++++++++++++++

If true, the ``hostdir`` will have subdirectories for each app. All apps will still have access to a shared mount point, however they will be in completely isolated subdirectories.

docker:pids-limit
+++++++++++++++++

Maximum number of pids in a single container. Defaults to unlimited.

Sample file
===========

Here is a complete example:

.. highlight:: yaml

::

    listen: "0.0.0.0:8080"
    debug: true
    host: http://<machine-public-addr>:8080 # This port must be the same as in the "listen" conf
    auth:
        user-registration: true
        scheme: native
    database:
        url: <your-mongodb-server>:27017
        name: tsurudb
    provisioner: docker
    docker:
        collection: docker_containers
        repository-namespace: tsuru
        deploy-cmd: /var/lib/tsuru/deploy
        cluster:
            storage: mongodb
            mongo-url: <your-mongodb-server>:27017
            mongo-database: cluster
        run-cmd:
            bin: /var/lib/tsuru/start
            port: "8888"
