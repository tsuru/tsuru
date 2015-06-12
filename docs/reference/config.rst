.. Copyright 2015 tsuru authors. All rights reserved.
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

use-tls
+++++++

``use-tls`` indicates whether tsuru should use TLS or not. This setting is
optional, and defaults to "false".

tls:cert-file
+++++++++++++

``tls:cert-file`` is the path to the X.509 certificate file configured to serve
the domain.  This setting is optional, unless ``use-tls`` is true.

tls:key-file
++++++++++++

``tls:key-file`` is the path to private key file configured to serve the
domain. This setting is optional, unless ``use-tls`` is true.

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
        <p>we're unsafe</p>
        {{end}}
    </body>

This setting is optional. When ``index-page-template`` is not defined, tsuru
will use the `default template
<https://github.com/tsuru/tsuru/blob/master/api/index_templates.go>`_.

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

database:logdb-url
++++++++++++++++++

This setting is optional. If ``database:logdb-url`` is specified, tsuru will use
it as the connection string to the MongoDB server responsible for storing
application logs. If this value is not set, tsuru will use ``database:url``
instead.

This setting is useful because tsuru may have to process a very large number of
log messages depending on the number of units deployed and applications
behavior. Every log message will trigger a insertion in MongoDB and this may
negatively impact the database performance. Other measures will be implemented
in the future to improve this, but for now, having the ability to use an
exclusive database server for logs will help mitigate the negative impact of log
writing.

database:logdb-name
+++++++++++++++++++

This setting is optional. If ``database:logdb-name`` is specified, tsuru will
use it as the database name for storing application logs. If this value is not
set, tsuru will use ``database:name`` instead.

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

Repository configuration
------------------------

tsuru optionally uses `Gandalf <https://github.com/tsuru/gandalf>`_ to manage
git repositories. Gandalf exposes a REST API for repositories management and
tsuru needs information about the Gandalf HTTP server endpoint.

repo-manager
++++++++++++

``repo-manager`` represents the repository manager that tsuru-server should use.
For backward compatibility reasons, the default value is "gandalf". Users can
disable repository and SSH key management by setting "repo-manager" to "none".
For more details, please refer to the :doc:`repository management page
</managing/repositories>` in the documentation.

git:api-server
++++++++++++++

``git:api-server`` is the address of the Gandalf API. It should define the
entire address, including protocol and port. Examples of value:
``http://localhost:9090`` and ``https://gandalf.tsuru.io:9595``.

Authentication configuration
----------------------------

tsuru has support for ``native`` and ``oauth`` authentication schemes.

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

.. _config_queue:

Queue configuration
-------------------

tsuru uses a work queue for asynchronous tasks.

``queue:*`` groups configuration settings for a MongoDB server that will be used
as storage for delayed execution of queued jobs.

This queue is used to manage creation and destruction of IaaS machines, but
tsuru may start using it in more places in the future.

It's not mandatory to configure the queue, however creating and removing
machines using a IaaS provider will not be possible.

queue:mongo-url
+++++++++++++++

Connection url for MongoDB server used to store task information.

queue:mongo-database
++++++++++++++++++++

Database name used in MongoDB. This value will take precedence over any database
name already specified in the connection url.

pubsub
++++++

``pubsub`` configuration is optional and depends on a redis server instance.
It's used only for following application logs (running ``tsuru app-log -f``). If
this is not configured tsuru will fail when running ``tsuru app-log -f``.

Previously the configuration for this redis server was inside ``redis-queue:*``
keys shown below. Using these keys is deprecated and tsuru will start ignoring
them before 1.0 release.

pubsub:redis-host
+++++++++++++++++

``pubsub:redis-host`` is the host of the Redis server to be used for pub/sub.
This settings is optional and defaults to "localhost".

pubsub:redis-port
+++++++++++++++++

``pubsub:redis-port`` is the port of the Redis server to be used for pub/sub.
This settings is optional and defaults to 6379.

pubsub:redis-password
+++++++++++++++++++++

``pubsub:redis-password`` is the password of the Redis server to be used for
pub/sub. This settings is optional and defaults to "", indicating that the Redis
server is not authenticated.

pubsub:redis-db
+++++++++++++++

``pubsub:redis-db`` is the database number of the Redis server to be used for
pub/sub. This settings is optional and defaults to 3.

pubsub:pool-max-idle-conn
+++++++++++++++++++++++++

``pubsub:pool-max-idle-conn`` is the maximum number of idle connections to
redis. Defaults to 20.

pubsub:pool-idle-timeout
++++++++++++++++++++++++

``pubsub:pool-idle-timeout`` is the number of seconds idle connections will
remain in connection pool to redis. Defaults to 300.

redis-queue:host
++++++++++++++++

Deprecated. See ``pubsub:redis-host``.

redis-queue:port
++++++++++++++++

Deprecated. See ``pubsub:redis-port``.

redis-queue:password
++++++++++++++++++++

Deprecated. See ``pubsub:redis-password``.

redis-queue:db
++++++++++++++

Deprecated. See ``pubsub:redis-db``.

.. _config_admin_user:

Admin users
-----------

tsuru has a very simple way to identify admin users: an admin user is a user
that is the member of the admin team, and the admin team is defined in the
configuration file, using the ``admin-team`` setting.

.. _config_admin_team:

admin-team
++++++++++

``admin-team`` is the name of the administration team for the current tsuru
installation. All members of the administration team is able to use the
``tsuru-admin`` command.

Quota management
----------------

tsuru can, optionally, manage quotas. Currently, there are two available
quotas: apps per user and units per app.

tsuru administrators can control the default quota for new users and new apps
in the configuration file, and use ``tsuru-admin`` command to change quotas for
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

routers:<router name>:type
++++++++++++++++++++++++++

Indicates the type of this router configuration. Currently only the value
``hipache`` is supported. tsuru also has an experimental router implementation
using `Galeb router <http://galeb.io/>`_ which is available using ``galeb`` type
value.

Depending on the type, there are some specific configuration options available.

routers:<router name>:redis-server (type: hipache)
++++++++++++++++++++++++++++++++++++++++++++++++++

Redis server used by Hipache router. This same server (or a redis slave of it),
must be configured in your hipache.conf file.

routers:<router name>:domain (type: hipache)
++++++++++++++++++++++++++++++++++++++++++++

The domain of the server running your hipache server. Applications created with
tsuru will have a address of ``http://<app-name>.<domain>``


routers:<router name>:api-url (type: galeb)
+++++++++++++++++++++++++++++++++++++++++++

The url for the Galeb manager api.

routers:<router name>:username (type: galeb)
++++++++++++++++++++++++++++++++++++++++++++

Galeb manager username.

routers:<router name>:password (type: galeb)
++++++++++++++++++++++++++++++++++++++++++++

Galeb manager password.

routers:<router name>:domain (type: galeb)
++++++++++++++++++++++++++++++++++++++++++

The domain of the server running your Galeb server. Applications created with
tsuru will have a address of ``http://<app-name>.<domain>``

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

Hipache
-------

hipache:redis-server
++++++++++++++++++++

Redis server used by Hipache router. This same server (or a redis slave of it),
must be configured in your hipache.conf file.

This setting is deprecated in favor of ``routers:<router name>:type = hipache``
and ``routers:<router name>:redis-server``.

hipache:domain
++++++++++++++

The domain of the server running your hipache server. Applications created with
tsuru will have a address of ``http://<app-name>.<hipache:domain>``.

This setting is deprecated in favor of ``routers:<router name>:type = hipache``
and ``routers:<router name>:domain``


Defining the provisioner
------------------------

tsuru has extensible support for provisioners. A provisioner is a Go type that
satisfies the `provision.Provisioner` interface. By default, tsuru will use
``DockerProvisioner`` (identified by the string "docker"), and now that's the
only supported provisioner (Ubuntu Juju was supported in the past but its
support has been removed from tsuru).

provisioner
+++++++++++

``provisioner`` is the string the name of the provisioner that will be used by
tsuru. This setting is optional and defaults to "docker".

Docker provisioner configuration
--------------------------------

docker:collection
+++++++++++++++++

Database collection name used to store containers information.

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
<docker:repository-namespace>/<app-name>

docker:max-workers
++++++++++++++++++

Maximum amount of threads to be created when starting new containers, so tsuru
doesn't start too much threads in the process of starting 1000 units, for
instance. Defaults to 0 which means unlimited.

.. _config_docker_router:

docker:router
+++++++++++++

Default router to be used to distribute requests to units. This should be the
name of a router configured under the ``routers:<name>`` key, see :ref:`routers
<config_routers>`.

For backward compatibility reasons, the value ``hipache`` is also supported, and
it will use either configuration available under ``router:hipache:*`` or
``hipache:*``, in this order.

Note that as of 0.10.0, routers may be associated to plans, if when creating an
application the chosen plan has a router value it will be used instead of the
value set in ``docker:router``.

The router defined in ``docker:router`` will only be used if the chosen plan
doesn't specify one.

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

docker:segregate
++++++++++++++++

Deprecated. As of tsuru 0.11.1, using segregate scheduler is the default
setting. See :doc:`/managing/segregate-scheduler` for details.

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

This setting, along with ``docker:scheduler:total-memory-metadata``, are also
used by node auto scaling. See :doc:`node auto scaling
</advanced_topics/node_scaling>` for more details.

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

The user tsuru will use to start the container. The value expected for
basebuilder platforms is ``ubuntu``.

docker:ssh:user
+++++++++++++++

Deprecated. You should set ``docker:user`` instead.

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

docker:healthcheck:max-time
+++++++++++++++++++++++++++

Maximum time in seconds to wait for deployment time health check to be
successful. Defaults to 120 seconds.

.. _config_image_history_size:

docker:image-history-size
+++++++++++++++++++++++++

Number of images available for rollback using ``tsuru app-deploy-rollback``.
tsuru will try to delete older images, but it may not be able to due to it being
used as a layer to a newer image. tsuru will keep trying to remove these old
images until they are not used as layers anymore. Defaults to 10 images.

.. _config_docker_auto_scale:

docker:auto-scale:enabled
+++++++++++++++++++++++++

Enable node auto scaling. See :doc:`node auto scaling
</advanced_topics/node_scaling>` for more details. Defaults to false.

docker:auto-scale:wait-new-time
+++++++++++++++++++++++++++++++

Number of seconds tsuru should wait for the creation of a new node during the
scaling up process. Defaults to 300 seconds (5 minutes).

docker:auto-scale:group-by-metadata
+++++++++++++++++++++++++++++++++++

Name of the metadata present in nodes that will be used for grouping nodes into
clusters. See :doc:`node auto scaling </advanced_topics/node_scaling>` for more
details. Defaults to empty (all nodes belong the the same cluster).

docker:auto-scale:metadata-filter
+++++++++++++++++++++++++++++++++

Value of the metadata specified by `docker:auto-scale:group-by-metadata`. If
this is set, tsuru will only run auto scale algorithms for nodes in the cluster
defined by this value.

docker:auto-scale:max-container-count
+++++++++++++++++++++++++++++++++++++

Maximum number of containers per node, for count based scaling. See :doc:`node
auto scaling </advanced_topics/node_scaling>` for more details.

docker:auto-scale:prevent-rebalance
+++++++++++++++++++++++++++++++++++

Prevent rebalancing from happening when adding new nodes, or if a rebalance is
needed. See :doc:`node auto scaling </advanced_topics/node_scaling>` for more
details.

docker:auto-scale:run-interval
++++++++++++++++++++++++++++++

Number of seconds between two periodic runs of the auto scaling algorithm.
Defaults to 3600 seconds (1 hour).

docker:auto-scale:scale-down-ratio
++++++++++++++++++++++++++++++++++

Ratio used when scaling down. Must be greater than 1.0. See :doc:`node auto
scaling </advanced_topics/node_scaling>` for more details. Defaults to 1.33.

.. _iaas_configuration:

IaaS configuration
==================

tsuru uses IaaS configuration to automatically create new docker nodes and
adding them to your cluster when using ``docker-node-add`` command. See
:doc:`adding nodes</installing/adding-nodes>` for more details about how to use
this command.

.. attention::

    You should configure :ref:`queue <config_queue>` to be able to use IaaS.


General settings
----------------

iaas:default
++++++++++++

The default IaaS tsuru will use when calling ``docker-node-add`` without
specifying ``iaas=<iaas_name>`` as a metadata. Defaults to ``ec2``.

iaas:node-protocol
++++++++++++++++++

Which protocol to use when accessing the docker api in the created node.
Defaults to ``http``.

iaas:node-port
++++++++++++++

In which port the docker API will be accessible in the created node. Defaults to
``2375``.

iaas:collection
+++++++++++++++

Collection name on database containing information about created machines.
Defaults to ``iaas_machines``.

EC2 IaaS
--------

iaas:ec2:key-id
+++++++++++++++

Your AWS key id.

iaas:ec2:secret-key
+++++++++++++++++++

Your AWS secret key.

iaas:ec2:user-data
++++++++++++++++++

A url for which the response body will be sent to ec2 as user-data.
Defaults to a script which will run `tsuru now installation
<https://github.com/tsuru/now>`_.

iaas:ec2:wait-timeout
+++++++++++++++++++++

Number of seconds to wait for the machine to be created. Defaults to 300 (5
minutes).

CloudStack IaaS
---------------

iaas:cloudstack:api-key
+++++++++++++++++++++++

Your api key.

iaas:cloudstack:secret-key
++++++++++++++++++++++++++

Your secret key.

iaas:cloudstack:url
+++++++++++++++++++

The url for the cloudstack api.

iaas:cloudstack:user-data
+++++++++++++++++++++++++

A url for which the response body will be sent to cloudstack as user-data.
Defaults to a script which will run `tsuru now installation
<https://github.com/tsuru/now>`_.

iaas:cloudstack:wait-timeout
++++++++++++++++++++++++++++

Number of seconds to wait for the machine to be created. Defaults to 300 (5
minutes).

.. _config_custom_iaas:

Custom IaaS
-----------

You can define a custom IaaS based on an existing provider. Any configuration
keys with the format ``iaas:custom:<name>`` will create a new IaaS with
``name``.

iaas:custom:<name>:provider
+++++++++++++++++++++++++++

The base provider name, it can be one of the supported providers: ``cloudstack``
or ``ec2``.

iaas:custom:<name>:<any_other_option>
+++++++++++++++++++++++++++++++++++++

This will overwrite the value of ``iaas:<provider>:<any_other_option>`` for this
IaaS. As an example, having the configuration below would allow you to call
``tsuru-admin docker-node-add iaas=region1_cloudstack ...``:

.. highlight:: yaml

::
    
    iaas:
        custom:
            region1_cloudstack:
                provider: cloudstack
                url: http://region1.url/
                secret-key: mysecretkey
        cloudstack:
            api-key: myapikey    


Sample file
===========

Here is a complete example:

.. highlight:: yaml

::

    listen: "0.0.0.0:8080"
    debug: true
    host: http://<machine-public-addr>:8080 # This port must be the same as in the "listen" conf
    admin-team: admin
    auth:
        user-registration: true
        scheme: native
    database:
        url: <your-mongodb-server>:27017
        name: tsurudb
    pubsub:
        redis-host: <your-redis-server>
        redis-port: 6379
    queue:
        mongo-url: <your-mongodb-server>:27017
        mongo-database: queuedb
    git:
        api-server: http://<your-gandalf-server>:8000
    provisioner: docker
    docker:
        router: hipache
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
    routers:
        hipache:
            type: hipache
            domain: <your-hipache-server-ip>.xip.io
            redis-server: <your-redis-server-with-port>
