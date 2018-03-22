.. Copyright 2016 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

:title: tsuru Installer
:description: Installs tsuru and its dependencies

.. _installer:

###############
tsuru Installer
###############

tsuru Installer provides a way to install tsuru API and it's required components
locally or on remote hosts.

.. note::

    tsuru Installer is distributed inside the tsuru client. To use it, you must
    first install the client. Check the tsuru client documentation for a full
    reference, including how to install it: https://tsuru-client.readthedocs.org.


To install tsuru locally, one can simply run
(requires `VirtualBox <https://www.virtualbox.org/wiki/Downloads>`_):

.. highlight:: bash

::

    $ tsuru install-create


After a couple of minutes you will have a full tsuru installation, inside a local
VirtualBox VM, where you can start deploying your applications and experience the
tsuru workflow.

How it works
============

tsuru installer uses `docker machine <https://www.docker.com/products/docker-machine>`_
to provision docker hosts, this means that it's possible to use any of the core or
3rd party docker machine drivers on the installation.

It will create a directory inside your ``~/.tsuru/installs``, with every file created
and needed by docker machine to manage and provision your hosts: certificates,
configuration files, your CA file etc.

After provisioning the hosts, the installer will install and start every tsuru
component as a swarm service on the hosts.

Docker Machine drivers
----------------------

Docker Machine drivers are responsible for provisioning docker hosts on different
iaas'. The installer comes bundled with all `docker machine core drivers <https://docs.docker.com/machine/drivers/>`_
and also supports the 3rd party ones; just make sure they are available in your $PATH.

For a list of 3rd party plugins supported by the community
check `here <https://github.com/docker/docker.github.io/blob/master/machine/AVAILABLE_DRIVER_PLUGINS.md>`_.

Swarm Mode
----------

tsuru installer provisions docker hosts with docker v1.12 and uses docker swarm mode
to orchestrate it's core components in the docker node cluster. This means that it's
easy to scale up and down every service and swarm is also responsible for recovering
a service if one of it's tasks is lost.

Hosts
-----

The installer provision and manages two kinds of hosts: core hosts and apps hosts.

Core hosts are Swarm nodes and are responsible for running tsuru core components as
swarm services (orchestrated by Swarm).

Apps hosts are docker hosts registered as docker nodes to tsuru. These are responsible
for running tsuru apps (orchestrated by tsuru).

By default, core hosts are reused as apps hosts (this can be configured by the ``hosts:apps:dedicated`` config).

What is installed
=================

Currently, the installer installs the following components:

* MongoDB
* Redis
* PlanB router
* Docker Registry
* tsuru API

After all basic components are installed, it will:

* Create a root user on tsurud
* Point your tsuru client to the newly created api using `tsuru target-set`
* Configure a docker node to run your applications
* Create and deploy a tsuru-dashboard


Security
========

The installer needs to issue commands to the tsuru api during the installation and,
to do so, it uses the ``--<driver-name>-open-port 8080/tcp`` driver flag, configuring the host
to have the 8080/tcp port opened to the internet. This is probably not recommended and should be changed as soon as possible after
the installation. For drivers that do not support this parameter, the port needs to be opened manually or
the corresponding driver flag must be set on the installation configuration file.

It is also recommended to change the root user login and password that the installer
uses to bootstrap the installation.

.. _customize:

Customizing the installation
============================

The ``install`` command accepts a configuration file as parameter to customize the
installation.

For example, to install tsuru on amazon ec2, one could create the following file:

.. highlight:: yaml

::

    driver:
      name: amazonec2
      options:
        amazonec2-access-key: myAmazonAccessKey
        amazonec2-secret-key: myAmazonSecretKey
        amazonec2-vpc-id: vpc-abc1234
        amazonec2-subnet-id: subnet-abc1234

And pass it to the install command as:

.. highlight:: bash

::

    $ tsuru install-create -c config.yml

.. _examples:

Examples
========

This section cover some examples to show some of the capabilities of the installer.

Multi-host provisioning and installation on AWS
-----------------------------------------------

The following configuration will provision 3 virtual machines on AWS to run tsuru
core components and other 3 machines to host tsuru applications. Additionaly,
it will use an external mongoDB instead of installing it.

.. highlight:: yaml

::

    components:
        mongo: mongoDB.my-server.com:27017
    hosts:
        core:
            size: 3
            driver:
                options:
                    amazonec2-zone: ["a", "b", "c"]
                    amazonec2-instance-type: "t2.medium"
        apps:
            size: 3
            dedicated: true
            driver:
                options:
                    amazonec2-zone: ["a", "b", "c"]
                    amazonec2-instance-type: "t2.small"
    driver:
        name: amazonec2
        options:
            amazonec2-access-key: myAmazonAccessKey
            amazonec2-secret-key: myAmazonSecretKey
            amazonec2-vpc-id: vpc-abc1234

Each core/apps host will be created in a different availability zone, "t2.medium" instances
will be provisioned for core hosts and "t2.small" for apps hosts.

Installing on already provisioned (or physical) hosts
-----------------------------------------------------

Docker machine provides a `generic driver <https://docs.docker.com/machine/drivers/generic/>`_
that can be used to install docker to already provisioned virtual or physical machines using ssh.
The following configuration example will connect to machine-1 and machine-2 using ssh,
install docker, install and start all tsuru core components on those two machines.
Machine 3 will be registered as an application node to be used by tsuru applications,
including the dashboard.

.. highlight:: yaml

::

    hosts:
        core:
            size: 2
            driver:
                options:
                    generic-ip-address: ["machine-1-IP", "machine-2-IP"]
                    generic-ssh-key: ["~/keys/machine-1", "~/keys/machine-2"]
        apps:
            size: 1
            dedicated: true
            driver:
                options:
                    generic-ip-address: ["machine-3-IP"]
                    generic-ssh-key: ["~/keys/machine-3"]
    driver:
        name: generic
        options:
            generic-ssh-port: 2222
            generic-ssh-user: ubuntu

DigitalOcean basic configuration
--------------------------------

For example, to install tsuru on DigitalOcean, one could create the following file:

.. highlight:: yaml

::

  driver:
    name: digitalocean
    options:
      digitalocean-access-token: your-token
      digitalocean-image: ubuntu-15-10-x64
      digitalocean-region: nyc3
      digitalocean-size: 512mb
      digitalocean-ipv6: false
      digitalocean-private-networking: false
      digitalocean-backups: false
      digitalocean-ssh-user: root
      digitalocean-ssh-port: 22
      digitalocean-ssh-key-fingerprint: the-ssh-key-fingerprint

Configuration reference
=======================

.. note::

    tsuru uses a colon to represent nesting in YAML. So, whenever this document says
    something like ``key1:key2``, it refers to the value of the ``key2`` that is
    nested in the block that is the value of ``key1``. For example,
    ``database:url`` means:

    .. highlight:: yaml

    ::

        database:
          url: <value>


name
----

The name of the installation, e.g, tsuru-ec2, tsuru-local. This will be the name
of the directory created inside ``~/.tsuru/installs`` and the tsuru target name
for the api.

components:<component>
----------------------

This configuration can be used to disable the installation of a core component,
by setting the component address. For example, by setting:

.. highlight:: yaml

::

    components:
      mongo: my-mongo.example.com:27017

The installer won't install the mongo component and instead will check the connection
to my-mongo.example.com:27017 before continuing with the installation.
The following components can be configured to be used as an external resource: mongo,
redis, registry and planb.

hosts:core:size
---------------

Number of machines to be used as hosts for tsuru core components. Default 1.

hosts:core:driver:options
-------------------------

Under this namespace every driver parameters can be set. These are going to be
used only for core hosts and each parameter accepts a list or a single value.
If the number of values is less than the number of hosts, some values will be
reused across the core hosts.

hosts:apps:size
---------------

Number of machines to be registered as docker nodes to host tsuru apps. Default 1.

hosts:apps:dedicated
--------------------

Boolean flag to indicated if apps hosts are dedicated or if they can be used
to run tsuru core components. Defaults to true.

hosts:apps:driver:options
-------------------------

Under this namespace every driver parameters can be set. These are going to be
used only for app hosts and each parameter accepts a list or a single value.
If the number of values is less than the number of hosts, some values will be
reused across the apps hosts.

docker-hub-mirror
-----------------

Url of a docker hub mirror used to fetch the components docker images. The default
is to use no mirror.

ca-path
-------

A path to a directory containing a ca.pem and ca-key.pem files that are going to
be used to sign certificates used by docker and docker registry. If not set,
one will be created.

driver:name
-----------

Name of the driver to be used by the installer. This can be any core or 3rd party
driver supported by docker machine. If a 3rd party driver name is used, it's binary
must be available on the user path. The default is to use virtualbox.

driver:options
--------------

Under this namespace every driver parameters can be set. Refer to the driver
configuration for more information on what parameter are available. For exemple,
the AWS docker machine driver accepts the ``--amazonec2-secret-key`` argument and
this can be set using ``driver:options:amazonec2-secret-key`` entry.
