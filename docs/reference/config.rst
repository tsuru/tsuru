.. Copyright 2014 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

+++++++++++++++++
Configuring tsuru
+++++++++++++++++

tsuru uses a configuration file in `YAML <http://www.yaml.org/>`_ format. This
document describes what each option means, and how it should look like.

Notation
========

tsuru uses a colon to represent nesting in YAML. So, whenever this document say
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

Database access
---------------

tsuru uses MongoDB as database manager, to store information about users, VM's,
and its components. Regarding database control, you're able to define to which
database server tsuru will connect (providing a `MongoDB connection string
<http://docs.mongodb.org/manual/reference/connection-string/>`_). The database
related options are listed below:

database:url
++++++++++++

``database:url`` is the database connection string. It is a mandatory setting
and has no default value. Examples of strings include the basic "127.0.0.1" and
the more advanced "mongodb://user@password:127.0.0.1:27017/database". Please
refer to `MongoDB documentation
<http://docs.mongodb.org/manual/reference/connection-string/>`_ for more
details and examples of connection strings.

database:name
+++++++++++++

``database:name`` is the name of the database that tsuru uses. It is a
mandatory setting and has no default value. An example of value is "tsuru".

Email configuration
-------------------

tsuru sends email to users when they request password recovery. In order to
send those emails, tsuru needs to be configured with some SMTP settings.
Omitting these settings won't break tsuru, but users would not be able to reset
their password automatically.

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

Git configuration
-----------------

tsuru uses `Gandalf <https://github.com/tsuru/gandalf>`_ to manage git
repositories. Gandalf exposes a REST API for repositories management, and tsuru
uses it. So tsuru requires information about the Gandalf HTTP server, and also
its git-daemon and SSH service.

tsuru also needs to know where the git repository will be cloned and stored in
units storage. Here are all options related to git repositories:

git:unit-repo
+++++++++++++

``git:unit-repo`` is the path where tsuru will clone and manage the git
repository in all units of an application. This is where the code of the
applications will be stored in their units. Example of value:
``/home/application/current``.


git:api-server
++++++++++++++

``git:api-server`` is the address of the Gandalf API. It should define the
entire address, including protocol and port. Examples of value:
``http://localhost:9090`` and ``https://gandalf.tsuru.io:9595``.

git:rw-host
+++++++++++

``git:rw-host`` is the host that will be used to build the push URL. For
example, when the value is "tsuruhost.com", the push URL will be something like
git@tsuruhost.com:<app-name>.git.

git:ro-host
+++++++++++

``git:ro-host`` is the host that units will use to clone code from users
applications. It's used to build the read only URL of the repository. For
example, when the value is "tsuruhost.com", the read-only URL will be something
like git://tsuruhost.com/<app-name>.git.

Authentication configuration
----------------------------

tsuru has support for ``native`` and ``oauth`` authentication schemes.

The default scheme is ``native`` and it supports the creation of users in
tsuru's internal database. It hashes passwords brcypt and tokens are generated
during authentication, and are hashed using SHA512.

The ``auth`` section also controls whether user registration is on or off. When
user registration is off, the user creation URL is not registered in the
server.

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

Every config entry inside ``auth:oauth`` are used when the ``auth:scheme`` is set
to "oauth". Please check `rfc6749 <http://tools.ietf.org/html/rfc6749>`_ for more
details.

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

The URL used in the authorization step of the OAuth flow. tsuru CLI will
receive this URL and trigger the opening a browser on this URL with the necessary
parameters.

During the authorization step, tsuru CLI will start a server locally and set the
callback to http://localhost:<port>, if ``auth:oauth:callback-port`` is set tsuru
CLI will use its value as <port>. If ``auth:oauth:callback-port`` isn't present
tsuru CLI will automatically choose an open port.

The callback URL should be registered on your OAuth server.

If the chosen server requires the callback URL to match the same host and port as
the registered one you should register "http://localhost:<chosen port>" and set
the ``auth:oauth:callback-port`` accordingly.

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

queue configuration
-------------------

tsuru uses a work queue for asynchronous tasks.

Currently, tsuru supports only ``redis`` as queue backend. Creating a new queue
provider is as easy as implementing `an interface
<http://godoc.org/github.com/tsuru/tsuru/queue#Q>`_.

queue
+++++

``queue`` is the name of the queue implementation that tsuru will use. This
setting defaults to ``redis``.

redis-queue:host
++++++++++++++++

``redis-queue:host`` is the host of the Redis server to be used for the working
queue. This settings is optional and defaults to "localhost".

redis-queue:port
++++++++++++++++

``redis-queue:port`` is the port of the Redis server to be used for the working
queue. This settings is optional and defaults to 6379.

redis-queue:password
++++++++++++++++++++

``redis-queue:password`` is the password of the Redis server to be used for the
working queue. This settings is optional and defaults to "", indicating that
the Redis server is not authenticated.

redis-queue:db
++++++++++++++

``redis-queue:db`` is the database number of the Redis server to be used
for the working queue. This settings is optional and defaults to 3.

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

Log level
---------

debug
+++++

``false`` is the default value, so you won't see any
noises on logs, to turn it on set it to true, e.g.: ``debug: true``

Hipache
-------

hipache:redis-server
++++++++++++++++++++

Redis server used by Hipache router. This same server (or a redis slave of it),
must be configured in your hipache.conf file.

hipache:domain
++++++++++++++

The domain of the server running your hipache server. Applications created with
tsuru will have a address of ``http://<app-name>.<hipache:domain>``


Defining the provisioner
------------------------

tsuru has extensible support for provisioners. A provisioner is a Go type that
satisfies the `provision.Provisioner` interface. By default, tsuru will use
``DockerProvisioner`` (identified by the string "docker"), and now that's the only
supported provisioner (Ubuntu Juju was supported in the past but its support has
been removed from tsuru).

provisioner
+++++++++++

``provisioner`` is the string the name of the provisioner that will be used by
tsuru. This setting is optional and defaults to "docker".

Docker provisioner configuration
--------------------------------

docker:collection
+++++++++++++++++

Database collection name used to store containers information.

docker:repository-namespace
+++++++++++++++++++++++++++

Docker repository namespace to be used for application and platform images. Images
will be tagged in docker as <docker:repository-namespace>/<platform-name> and
<docker:repository-namespace>/<app-name>

docker:router
+++++++++++++

Router to be used to distribute requests to units. Right now only ``hipache`` is
supported.

docker:deploy-cmd
+++++++++++++++++

The command that will be called in your platform when a new deploy happens.
The default value for platforms supported in tsuru's basebuilder repository is
``/var/lib/tsuru/deploy``.

docker:segregate
++++++++++++++++

Enable segregate scheduler. See :doc:`/managing/segregate-scheduler` for details.

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

The command that will be called on the application image to start the application.
The default value for platforms supported in tsuru's basebuilder repository is
``/var/lib/tsuru/start``.

docker:run-cmd:port
+++++++++++++++++++

The tcp port that will be exported by the container to the node network. The
default value expected by platforms defined in tsuru's basebuilder repository is
``8888``.

docker:ssh:add-key-cmd
++++++++++++++++++++++

The command that will be called with the ssh public key created for the
application. This allows us to connect directly to a running container using ssh.
The value expected for basebuilder platforms is ``/var/lib/tsuru/add-key``.

docker:ssh:public-key
+++++++++++++++++++++

Deprecated. You shouldn't set this value anymore.

docker:ssh:user
+++++++++++++++

The user used to connect via ssh to running containers. The value expected for
basebuilder platforms is ``ubuntu``.

.. _iaas_configuration:

IaaS configuration
------------------

iaas:default
++++++++++++

Define the default IaaS to tsuru use to create/list/delete your nodes.

iaas:api-key
++++++++++++

Api-key to authenticate on IaaS.

iaas:secret-key
+++++++++++++++

Secret-key to authenticate on IaaS.

iaas:url
++++++++

Endpoint API to use the IaaS

iaas:node-protocol
++++++++++++++++++

Protocol to create node URL.

iaas:node-port
++++++++++++++

Port to create node URL.

iaas:collection
+++++++++++++++

Collection to handle machine data on database.

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
    queue: redis
    redis-queue:
        host: <your-redis-server>
        port: 6379
    git:
        unit-repo: /home/application/current
        api-server: http://<your-gandalf-server>:8000
    provisioner: docker
    docker:
        segregate: false
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
        ssh:
            add-key-cmd: /var/lib/tsuru/add-key
            user: ubuntu
    hipache:
        domain: <your-hipache-server-ip>.xip.io
        redis-server: <your-redis-server-with-port>
