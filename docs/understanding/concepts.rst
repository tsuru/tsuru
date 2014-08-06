Concepts
========

Docker
------

Docker is an open source project to pack, ship and run any application as a
lightweight, portable, self-sufficient container.
When you deploy an app with git push, tsuru builds an create Docker image,
then distributes it as units across your cluster.

Clusters
--------

A cluster is a named group of nodes. `tsuru API` has a scheduler algorithm that
distribute applications intelligently across a cluster nodes.

Nodes
-----

Node is a physical or virtual machine with docker.

Applications
------------

An application, is a program's source code, dependencies list - on
operational system and language level - and a Procfile with instructions on how
to run that program. An application has a name, a unique address, a Platform,
associated development teams, a repository and a set of units.

Units
-----

A unit is a container. A unit has everything an application needs to run, the
fetched operational system and language level dependencies, the application's
source code, the language runtime, and the applications processes defined on
the Procfile.

Platforms
---------

A platform is a well defined pack with installed dependencies for a language or
framework that a group of applications will need. A platform might be a
container template (docker image).

For instance, tsuru has a container image for python applications, with
virtualenv installed and other required things needed for tsuru to deploy
applications on top of that platform. Platforms are easily extendable and
managed by tsuru. Every application runs on top of a platform.

Services
--------

A service is a well defined API that tsuru communicates with to provide extra
functionality for applications. Examples of services are MySQL, Redis, MongoDB,
etc. tsuru has built-in services, but it is easy to create and add new services
to tsuru. Services aren't managed by tsuru, but by its creators.
