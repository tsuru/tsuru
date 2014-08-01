Concepts
========

Twelve-Factor
-------------

Docker
------

Clusters
--------

Nodes
-----

Applications
------------

An application, is a program's source code, dependencies list - on
operational system and language level - and a Procfile with instructions on how
to run that program. An application has a name, a unique address, a Platform,
associated development teams, a repository and a set of units.

Units
-----

A unit is an isolated Unix container or a virtual machine - depending on the
configured provisioner. A unit has everything an application needs to run, the
fetched operational system and language level dependencies, the application's
source code, the language runtime, and the applications processes defined on
the Procfile.

Platforms
---------

A platform is a well defined pack with installed dependencies for a language or
framework that a group of applications will need. A platform might be a
container template, or a virtual machine image.

For instance, tsuru has a container image for python applications, with
virtualenv installed and other required things needed for tsuru to deploy
applications on top of that platform. Platforms are easily extendable in
tsuru, but currently not managed by it, all tsuru does (by now) is to keep
database records for each existent platform. Every application runs on top of
a platform.

Services
--------

A service is a well defined API that tsuru communicates with to provide extra
functionality for applications. Examples of services are MySQL, Redis, MongoDB,
etc. tsuru has built-in services, but it is easy to create and add new services
to tsuru. Services aren't managed by tsuru, but by its creators.

Check the :doc:`service usage documentation </reference/services>` for more
on using services and the :doc:`building your own service tutorial
</services/build>` for a quick start on how to extend tsuru by creating new
services.
