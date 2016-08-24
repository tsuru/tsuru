.. Copyright 2016 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

:title: tsuru Installer
:description: Installs tsuru and it`s dependencies

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

    $ tsuru install


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
component as a docker container on those hosts.

Docker Machine drivers
----------------------

Docker Machine drivers are responsible for provisioning docker hosts on different
iaas'. The installer comes bundled with all `docker machine core drivers <https://docs.docker.com/machine/drivers/>`_
and also supports the 3rd party ones; just make sure they are available in your $PATH.

For a list of 3rd party plugins supported by the community
check `here <https://github.com/docker/machine/blob/master/docs/AVAILABLE_DRIVER_PLUGINS.md>`_.


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
to do so, the host is configured to have the 8080/tcp port opened to the internet.
This is probably not recommended and should be changed as soon as possible after
the installation.

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

    $ tsuru install -c config.yml

Configuration reference
============================

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
