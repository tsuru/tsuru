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

tls-cert-file
+++++++++++

``tlscert-file`` is the path to the X.509 certificate file configured to serve
the domain.  This setting is optional, unless ``use-tls`` is true.

tls-key-file
++++++++++++

``tls-key-file`` is the path to private key file configured to serve the
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

Git configuration
-----------------

Tsuru uses `Gandalf <https://github.com/globocom/gandalf>`_ to manage git
repositories. Gandalf exposes a REST API for repositories management, and tsuru
uses it. So tsuru requires information about the Gandalf HTTP server.

Tsuru also needs to know where the git repository will be cloned and stored in
units storage. Here are all options related to git repositories:

git:unit-repo
+++++++++++++

``git:unit-repo`` is the path where tsuru will clone and manage the git
repository in all units of an application. This is where the code of the
applications will be stored in their units. Example of value:
``/home/application/current``.


git:host
++++++++

``git:host`` is the host for the Gandalf API. It should include the host name
only, not the schema nor the port. This setting is mandatory and has no default
value. Examples of value: ``localhost`` and ``gandalf.tsuru.io``.

git:port
++++++++

``git:port`` is the port for the Gandalf API. Its value must be a positive
integer. This setting is optional and defaults to "80".

git:protocol
++++++++++++

``git:protocol`` is the protocol to communicate with Gandalf API. The value may
be ``http`` or ``https``, all lower cased. This setting is optional and
defaults to "http".

Authentication configuration
----------------------------

Tsuru has its own authentication mechanism, that hashes passwords using SHA512,
PBKDF2 and salt. It also uses SHA512 for hashing tokens, generated during
authentication.

This mechanism requires three settings to operate: ``auth:salt``,
``auth:token-expire-days`` and ``auth:token-key``. Each setting is described
below:

auth:salt
+++++++++

``auth:salt`` is the salt used by tsuru when hashing password. This value is
optional and defaults to "tsuru-salt". This value affects all passwords, so *if
it change at anytime, all password must be regenerated*.

auth:token-expire-days
++++++++++++++++++++++

Whenever a user logs in, tsuru generates a token for him/her, and the user may
store the token. ``auth:token-expire-days`` setting defines the amount of days
that the token will be valid. This setting is optional, and defaults to "7".

auth:token-key
++++++++++++++

``auth:token-key`` is the key used for token hashing, during authentication
process. If this value changes, all tokens will expire. This setting is
optional, and defaults to "tsuru-key".

Amazon Web Services (AWS) configuration
---------------------------------------

Tsuru uses Amazon Web Services (AWS) Simple Storage Service (S3) to provide
static storage for apps. In the process of app creation, tsuru creates a S3
bucket and AWS Identity and Access Management (IAM) credentials to access this
bucket. In order to be able to comunicate with AWS API's, tsuru needs some
settings, listed below.

For more details on AWS authentication, AWS AIM and AWS S3, check AWS docs:
https://aws.amazon.com/documentation/.

aws:access-key-id
+++++++++++++++++

``aws:access-key-id`` is the access key ID used by tsuru to authenticate with
AWS API. This setting is required and has no default value.

aws:secret-access-key
+++++++++++++++++++++

``aws:secret-access-key`` is the secret access key used by tsuru to
authenticate with AWS API. This setting is required and has no default value.

aws:iam:endpoint
++++++++++++++++

``aws:iam:endpoint`` is the IAM endpoint that tsuru will call to create
credentials for its applications. This setting is optional, and defaults to
``https://iam.amazonaws.com/``. You should change this setting only when using
another service that also implements IAM's API.

aws:s3:region-name
++++++++++++++++++

``aws:s3:region-name`` is the name of the region that tsuru will use to create
S3 buckets. This setting is required and has no default value.

aws:s3:endpoint
+++++++++++++++

``aws:s3:endpoint`` is the S3 endpoint that tsuru will call to create buckets
for its applications. This setting is required and has no default value.

aws:s3:location-constraint
++++++++++++++++++++++++++

``aws:s3:location-constraint`` indicates whether buckets should be stored in
the selected region. This setting is required and has no default value.

For more details, check the documentation for buckets and regions:
http://docs.aws.amazon.com/AmazonS3/latest/dev/LocationSelection.html.

aws:s3:lowercase-bucket
+++++++++++++++++++++++

``aws:s3:lowercase-bucket`` will be true if the region requires bucket names to
be lowercase. This setting is required and has no default value.

provisioner
+++++++++++

Tsuru support multiple provisioner. A provisioner is a Go type that satisfies
an interface. By default, tsuru will use ``JujuProvisioner``. To use other
provisioner, that has been already registered with tsuru, one must define the
setting ``provisioner``. This setting is optional and defaults to "juju".

queue-server
++++++++++++

Tsuru uses `beanstalkd <http://kr.github.com/beanstalkd>`_ as a work queue.
``queue-server`` is the TCP address where beanstalkd is listening. This setting
is optional and defaults to "localhost:11300".

admin-team
++++++++++

``admin-team`` is the name of the administration team for the current tsuru
installation. All members of the administration team is able to use the
``tsuru-admin`` command.

Juju provisioner configuration
==============================

    PENDING. See `Issue 263 <https://github.com/globocom/tsuru/issues/263>`_
    for details.

Sample file
===========

Here is a complete example, with VPC, HTTP/TLS and load balacing enabled:

.. highlight:: yaml

::

    listen: ":8080"
    use-tls: true
    tls-cert-file: /etc/tsuru/tls/cert.pem
    tls-key-file: /etc/tsuru/tls/key.pem
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
      salt: salt
      token-expire-days: 14
      token-key: key
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
      use-elb: true
      elb-use-vpc: true
      elb-endpoint: https://elasticloadbalancing.amazonaws.com
      elb-vpc-subnets:
        - subnet-a1a1a1
      elb-vpc-secgroups:
        - sg-a1a1a1
      elb-collection: j_lbs
      units-collection: j_units
