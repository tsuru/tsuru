Concepts
========

Docker
------

`Docker <https://www.docker.com/>`_ is an open source project to pack, ship,
and run any application as a lightweight, portable, self-sufficient container.
When you deploy an app with ``git push`` or ``tsuru app-deploy``, tsuru builds
a Docker image and then distributes it as `units` (Docker containers) across
your cluster.

Clusters
--------

A cluster is a named group of nodes. `tsuru API` has a scheduler algorithm that
distributes applications intelligently across a cluster of nodes.

.. _concepts_nodes:

Nodes
-----

A node is a physical or virtual machine with Docker installed.

Managed node
++++++++++++

A managed node is a node created and managed by tsuru, using :ref:`IaaS
integration <iaas_configuration>`. tsuru manages this node, i.e. tsuru can heal
and scale it.

Unmanaged node
++++++++++++++

An unmanaged node is a node created manually, and just registered with tsuru.
tsuru is not able to manage these nodes, and it should be handled by whoever
created it manually.

Applications
------------

An application consists of:

- the program's source code - e.g.: Python, Ruby, Go, PHP, JavaScript, Java, etc.
- an operating system dependencies list -- in a file called ``requirements.apt``
- a language-level dependencies list -- e.g.: ``requirements.txt``, ``Gemfile``, etc.
- instructions on how to run the program -- in a file called ``Procfile``

An application has a name, a unique address, a platform, associated development
teams, a repository, and a set of units.

Units
-----

A unit is a container. A unit has everything an application needs to run; the
fetched operational system and language level dependencies, the application's
source code, the language runtime, and the application's processes defined in
the ``Procfile``.

Platforms
---------

A platform is a well-defined pack with installed dependencies for a language or
framework that a group of applications will need. A platform might be a
container template (Docker image).

For instance, tsuru has a container image for Python applications, with
virtualenv installed and other required things needed for tsuru to deploy
applications on top of that platform. Platforms are easily extendable and
managed by tsuru. Every application runs on top of a platform.

Services
--------

A service is a well-defined API that tsuru communicates with to provide extra
functionality for applications. Examples of services are MySQL, Redis, MongoDB,
etc. tsuru has built-in services, but it is easy to create and add new services
to tsuru. Services aren't managed by tsuru, but by their creators.
