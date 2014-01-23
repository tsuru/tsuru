.. Copyright 2013 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

+++++++++++++++++
Configuring tsuru
+++++++++++++++++

Tsuru uses a configuration file in `YAML <http://www.yaml.org/>`_ format. This
document describes what each option means, and how it should look like.

Notation
========

Tsuru uses a colon to represent nesting in YAML. So, whenever this document say
something like ``key1:key2``, it refers to the value of the ``key2`` that is
nested in the block that is the value of ``key1``. For example,
``database:url`` means:

.. highlight:: yaml

::

    database:
      url: <value>

Tsuru configuration
===================

This section describes tsuru's core configuration. Other sections will include
configuration of optional components, and finally, a full sample file.

HTTP server
-----------

Tsuru provides a REST API, that supports HTTP and HTTP/TLS (a.k.a. HTTPS). Here
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

Tsuru uses MongoDB as database manager, to store information about users, VM's,
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

Collector
---------

Collector is a tsuru agent responsible for collecting information about app units,
interacting with the provisioner. This agent runs a loop in configurable interval.

collector:ticker-time
+++++++++++++++++++++

``collector:ticker-time`` is interval for running the loop, specified in seconds.
Default value: 60 seconds.

Email configuration
-------------------

Tsuru sends email to users when they request password recovery. In order to
send those emails, Tsuru needs to be configured with some SMTP settings.
Omiting these settings won't break Tsuru, but users would not be able to reset
their password automattically.

smtp:server
+++++++++++

The SMTP server to connect to. It must be in the form <host>:<port>. Example:
"smtp.gmail.com:587".

smtp:user
+++++++++

The user to authenticate with the SMTP sever. Currently, Tsuru requires
authenticated sessions.

smtp:password
+++++++++++++

The password for authentication within the SMTP server.

Git configuration
-----------------

Tsuru uses `Gandalf <https://github.com/globocom/gandalf>`_ to manage git
repositories. Gandalf exposes a REST API for repositories management, and tsuru
uses it. So tsuru requires information about the Gandalf HTTP server, and also
its git-daemon and SSH service.

Tsuru also needs to know where the git repository will be cloned and stored in
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

Tsuru has its own authentication mechanism, that hashes passwords brcypt.
Tokens are generated during authentication, and are hashed using SHA512.

This mechanism requires two settings to operate: ``auth:hash-cost`` and
``auth:token-expire-days``. Each setting is described below.

The ``auth`` section also controls whether user registation is on or off. When
user registration is off, the user creation URL is not registered in the
server.

auth:user-registration
++++++++++++++++++++++

This flag indicates whether user registration is enabled. This setting is
optional, and defaults to false.

auth:hash-cost
++++++++++++++

This number indicates how many CPU time you're willing to give to hashing
calculation. It is an absolute number, between 4 and 31, where 4 is faster and
less secure, while 31 is very secure and *very* slow.

auth:token-expire-days
++++++++++++++++++++++

Whenever a user logs in, tsuru generates a token for him/her, and the user may
store the token. ``auth:token-expire-days`` setting defines the amount of days
that the token will be valid. This setting is optional, and defaults to "7".

auth:max-simultaneous-sessions
++++++++++++++++++++++++++++++

Tsuru can limit the number of simultaneous sessions per user. This setting is
optional, and defaults to "unlimited".

Amazon Web Services (AWS) configuration
---------------------------------------

Tsuru is able to use Amazon Web Services (AWS) Simple Storage Service (S3) to
provide static storage for apps. Whenever ``bucket-support`` is true, Tsuru
will create a S3 bucket and AWS Identity and Access Management (IAM)
credentials to access this bucket during the app creation process. In order to
be able to comunicate with AWS API's, tsuru needs some settings, listed below.

For more details on AWS authentication, AWS AIM and AWS S3, check AWS docs:
https://aws.amazon.com/documentation/.

bucket-support
++++++++++++++

``bucket-support`` is a boolean flag, that turns on the bucket per app feature.
This field is optional, and defaults to false.

aws:access-key-id
+++++++++++++++++

``aws:access-key-id`` is the access key ID used by tsuru to authenticate with
AWS API. Given that ``bucket-support`` is true, this setting is required and
has no default value.

aws:secret-access-key
+++++++++++++++++++++

``aws:secret-access-key`` is the secret access key used by tsuru to
authenticate with AWS API. Given that ``bucket-support`` is true, this
setting is required and has no default value.

aws:ec2:endpoint
++++++++++++++++

``aws:ec2:endpoint`` is the EC2 endpoint that tsuru will call to communicate
with ec2. It's only used for `juju` healers.

aws:iam:endpoint
++++++++++++++++

``aws:iam:endpoint`` is the IAM endpoint that tsuru will call to create
credentials for its applications. This setting is optional, and defaults to
``https://iam.amazonaws.com/``. You should change this setting only when using
another service that also implements IAM's API.

aws:s3:region-name
++++++++++++++++++

``aws:s3:region-name`` is the name of the region that tsuru will use to create
S3 buckets. Given that ``bucket-support`` is true, this setting is required and
has no default value.

aws:s3:endpoint
+++++++++++++++

``aws:s3:endpoint`` is the S3 endpoint that tsuru will call to create buckets
for its applications. Given that ``bucket-support`` is true, this setting is
required and has no default value.

aws:s3:location-constraint
++++++++++++++++++++++++++

``aws:s3:location-constraint`` indicates whether buckets should be stored in
the selected region. Given that ``bucket-support`` is true, this setting is
required and has no default value.

For more details, check the documentation for buckets and regions:
http://docs.aws.amazon.com/AmazonS3/latest/dev/LocationSelection.html.

aws:s3:lowercase-bucket
+++++++++++++++++++++++

``aws:s3:lowercase-bucket`` will be true if the region requires bucket names to
be lowercase. Given that ``bucket-support`` is true, this setting is required
and has no default value.

queue configuration
-------------------

Tsuru uses a work queue for asynchronous tasks. By default it will use
`beanstalkd <http://kr.github.com/beanstalkd>`_. You can customize the used
queue, and settings related to the queue (like the address where beanstalkd is
listening).

Besides beanstalkd, Tsuru also supports Redis as a working queue. In order to
use Redis, Tsuru administrators must set ``queue`` to ``redis``.

Creating a new queue provider is as easy as implementing `an interface
<http://godoc.org/github.com/globocom/tsuru/queue#Q>`_.

queue
+++++

``queue`` is the name of the queue implementation that tsuru will use. This
setting is optional and defaults to "beanstalkd".

queue-server
++++++++++++

``queue-server`` is the TCP address where beanstalkd is listening. This setting
is optional and defaults to "localhost:11300".

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

``redis-queue:password`` is the database number of the Redis server to be used
for the working queue. This settings is optional and defaults to 3.

Admin users
-----------

Tsuru has a very simple way to identify admin users: an admin user is a user
that is the member of the admin team, and the admin team is defined in the
configuration file, using the ``admin-team`` setting.

admin-team
++++++++++

``admin-team`` is the name of the administration team for the current tsuru
installation. All members of the administration team is able to use the
``tsuru-admin`` command.

Quota management
----------------

Tsuru can, optionally, manage quotas. Currently, there are two available
quotas: apps per user and units per app.

Tsuru administrators can control the default quota for new users and new apps
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

Defining the provisioner
------------------------

Tsuru supports multiple provisioners. A provisioner is a Go type that satisfies
an interface. By default, tsuru will use ``JujuProvisioner`` (identified by the
string "juju"). To use other provisioner, that has been already registered with
tsuru, one must define the setting ``provisioner``.

provisioner
+++++++++++

``provisioner`` is the string the name of the provisioner that will be used by
tsuru. This setting is optional and defaults to "juju".

You can also configure the provisioner (check the next section for details on
Juju configuration).

Juju provisioner configuration
==============================

"juju" is the default provisioner used by Tsuru. It's named after the `tool
used by tsuru <https://juju.ubuntu.com/>`_ to provision and manage instances.
It's a extended version of Juju, supporting Amazon's `Virtual Private Cloud
(VPC) <https://aws.amazon.com/vpc/>`_ and `Elastic Load Balancing (ELB)
<https://aws.amazon.com/elasticloadbalancing/>`_.

Charms path
-----------

Juju describe services as `Charms <http://jujucharms.com/>`_. Each tsuru
platform is a Juju charm. The tsuru team provides a collection of charms with
customized hooks: https://github.com/globocom/charms. In order (for more
details, refer to :doc:`build documentation </build>`).

juju:charms-path
++++++++++++++++

``charms-path`` is the path where tsuru should look for charms when creating
new apps. If you specify the value "/etc/juju/charms", your charms tree should
look something like this:

::

    .
    ├── centos
    │   ├── ...
    └── precise
        ├── go
        │   ├── config.yaml
        │   ├── hooks
        │   ...
        │   └── metadata.yaml
        ├── nodejs
        │   ├── config.yaml
        │   ├── hooks
        │   ...
        │   └── metadata.yaml
        ├── python
        │   ├── config.yaml
        │   ├── hooks
        │   ...
        │   ├── metadata.yaml
        │   └── utils
        │       ├── circus.ini
        │       └── nginx.conf
        ├── rack
        │   ├── config.yaml
        │   ├── hooks
        │   ...
        │   ├── metadata.yaml
        ├── ruby
        │   ├── config.yaml
        │   ├── hooks
        │   ...
        │   └── metadata.yaml
        └── static
            ├── config.yaml
            ├── hooks
            ...
            └── metadata.yaml

Given that you're using juju, this setting is mandatory and has no default
value.

Storing units in the database
-----------------------------

Juju provisioner uses the database to store information about units. It uses a
MongoDB collection that will be located in the same database used by tsuru. One
can set the name of this collection using the setting described below:

juju:units-collection
+++++++++++++++++++++

``juju:units-collection`` defines the name of the collection that Juju
provisioner should use to store information about units. This setting is
required by the provisioner and has no default value.

Elastic Load Balancing support
------------------------------

Juju provisioner can manage load balancers per app using Elastic Load Balancing
(ELB) API, provided by Amazon. In order to enable Elastic Load Balancing
support, one must set ``juju:use-elb`` to true and define other settings
described below:

juju:use-elb
++++++++++++

``juju:use-elb`` is a boolean flag that indicates whether Juju provisioner will
use ELB. When enabled, it will create a load balancer per app, registering and
deregistering units as they come and go, and deleting the load balancer when
the app is removed. This setting is optional and defaults to false.

Whenever ``juju:use-elb`` is defined to be true, other settings related to load
balancing become mandatory: ``juju:elb-endpoint``, ``juju:elb-collection``,
``juju:elb-avail-zones`` (or ``juju:elb-vpc-subnets`` and
``juju:elb-vpc-secgroups``, see ``juju:elb-use-vpc`` for more details).

juju:elb-endpoint
+++++++++++++++++

``juju:elb-endpoint`` is the ELB endpoint that tsuru will use to manage load
balancers. This setting has no default value, and is mandatory once
``juju:use-elb`` is true. When ``juju:use-elb`` is false, the value of this
setting is irrelevant.

juju:elb-collection
+++++++++++++++++++

``juju:elb-collection`` is the name of the collection that Juju provisioner
will use to store information about load balancers.

This setting has no default value, and is mandatory once ``juju:use-elb`` is
true. When ``juju:use-elb`` is false, the value of this setting is irrelevant.

juju:elb-use-vpc
++++++++++++++++

``juju:elb-use-vpc`` is another boolean flag. It indicates whether load
balancers should be created using an Amazon Virtual Private Cloud. When this
setting is true, one must also define ``juju:elb-vpc-subnets`` and
``juju:elb-vpc-secgroups``.

This setting is optional, defaults to false and has no effect when
``juju:use-elb`` is false.

juju:elb-vpc-subnets
++++++++++++++++++++

``juju:elb-vpc-subnets`` contains a list of subnets that will be attached to
the load balancer. This setting must be defined whenever ``juju:elb-use-vpc``
is true. It has no default value.

juju:elb-vpc-secgroups
++++++++++++++++++++++

``juju:elb-vpc-secgroups`` contains a list of security groups from which the
load balancer will inherit rules. This setting must be defined whenever
``juju:elb-use-vpc`` is true. It has no default value.

juju:elb-avail-zones
++++++++++++++++++++

``juju:elb-avail-zones`` contains a list of availability zones that the load
balancer will communicate with. This setting has no effect when
``juju:elb-use-vpc`` is true, has no default value and must be defined whenever
``juju:elb-use-vpc`` is false.

Sample file
===========

Here is a complete example, with S3, VPC, HTTP/TLS and load balacing enabled:

.. highlight:: yaml

::

    listen: ":8080"
    use-tls: true
    tls:
      cert-file: /etc/tsuru/tls/cert.pem
      key-file: /etc/tsuru/tls/key.pem
    host: http://10.19.2.238:8080
    database:
      url: 127.0.0.1:27017
      name: tsuru
    git:
      unit-repo: /home/application/current
      host: gandalf.tsuru.io
      port: 8000
      protocol: http
    auth:
      token-expire-days: 14
    bucket-support: true
    aws:
      access-key-id: access-key
      secret-access-key: s3cr3t
      iam:
        endpoint: https://iam.amazonaws.com/
      s3:
        region-name: sa-east-1
        endpoint: https://s3.amazonaws.com
        location-constraint: true
        lowercase-bucket: true
    provisioner: juju
    queue-server: "127.0.0.1:11300"
    admin-team: admin
    juju:
      charms-path: /etc/juju/charms
      units-collection: j_units
      use-elb: true
      elb-endpoint: https://elasticloadbalancing.amazonaws.com
      elb-collection: j_lbs
      elb-use-vpc: true
      elb-vpc-subnets:
        - subnet-a1a1a1
      elb-vpc-secgroups:
        - sg-a1a1a1
