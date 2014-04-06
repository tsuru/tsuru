Tsuru Overview
==============

This document is in alpha state, to suggest improvements check out the
`related github issue <https://github.com/tsuru/tsuru/issues/367>`_.

Tsuru is an open source PaaS. If you don't know what a PaaS is and what it
does, see `wikipedia's description <http://en.wikipedia.org/wiki/PaaS>`_.

It follows the principles described in the `The Twelve-Factor App
<http://www.12factor.net/>`_ methodology.

Fast and easy deployment
------------------------

Deploying an app is simple and easy. No special tools needed, just a plain git
push. The entire process is very simple, especially from the second deployment,
whether your app is big or small.

Tsuru uses git as the means of deploying an application. You don't need master
git in order to deploy an app to Tsuru, although you will need to know the very
basic workflow, add/commit/push and remote managing. Git allows really fast
deploys, and Tsuru makes the best possible use of it by not cloning the whole
repository history of your application, there's no need to have that
information in the application webserver.

Tsuru will also take care of all the applications dependencies in the
deployment process. You can specify operating system and language specific
dependencies. For example, if you have a Python application, tsuru will search
for the requirements.txt file, but first it will search for OS dependencies (a
list of deb packages in a file named requirements.apt, in the case of Ubuntu).

Tsuru also has :doc:`hooks </apps/deploy-hooks>` that can trigger commands
before and after some events that happen during the deployment process, like
restart (represented by ``restart:before``, ``restart:before-each``,
``restart:after`` and ``restart:after-each`` hooks).

Continuous Deployment
---------------------

Easily create testing, staging, and production versions of your app and deploy
to them instantly.

Add-on Resources
----------------

Instantly provision and integrate third party services with one command. Tsuru
provides the basic services your application will need, like searching,
caching, storage and frontend; you can get all of that in a fashionable and
really easy way using Tsuru's command line.

Per-Environment Config Variables
--------------------------------

Configuration for an application should be stored in environment variables -
and we know that. Tsuru lets you define your environment variables using the
command line, so you can have the configuration flexibility your application
need.

Tsuru also makes use of environment variables. When you bind a service with
your application, Tsuru gives the service the ability to inject environment
variables in your application environment. For instance, if you use the default
MySQL service, it will inject variables for you to establish a connection with
your application database.

Custom Services
---------------

Tsuru already has services for you to use, but you don't need to use them at
all if you don't want to. If you already have, let's say, a MySQL server
running on your infrastructure, all you need to do in order to use it is simply
configure environment variables and use them in your application config.

You can also create your own services and make them available for you and
others to use it on Tsuru. It's so easy to do so that you'll want to sell your
own services. Tsuru talks with services using a well defined `API
<https://tsuru.readthedocs.org/en/latest/services/api.html>`_, all you have to
do is implement four endpoints that knows how to provision instances of your
services and bind them to tsuru applications (like creating VMs, authorizing
security groups, creating ACLs, etc), and register your service in Tsuru with a
really simple `yaml manifest
<https://tsuru.readthedocs.org/en/latest/services/usage.html#crane-usage>`_.

Logging and Visibility
----------------------

Full visibility into your app's operations with real-time logging, process
status inspection, and an audit trail of all releases. Tsuru will capture
standard streams (output and error) from your application and expose them via
the ``tsuru log`` command. You can also filter logs, for example, if you don't
want to see the logs of developers activity (e.g.: a deploy action), you can
specify the source as "app" and you'll get only the application webserver logs.

Process Management
------------------

Tsuru manages all processes from an application, so you don't have to worry
about it. But it does not know to start it. You'll have to teach Tsuru how to
start your application using a Procfile. Tsuru reads the Procfile and uses
Circus_ to start and manage the running process. You can even enable a web
console for Circus to manage your application process and to watch CPU and
memory usage in real-time through a web interface.

Tsuru also allows you to easily restart your application process via command
line. Although Tsuru will do all the hard work of managing and fixing eventual
problems with your process, you might need to restart your application
manually, so we give you an easy way to do it.

.. _Circus: http://circus.readthedocs.org

Control Surfaces
----------------

Tsuru exposes its features through a solid, stable REST API. You can write
clients for this API, or you can use one of the clients maintained by tsuru
developers.

Tsuru ships with two API clients: the command line interface (CLI), which is
pretty stable and ready for day-to-day usage; and the `web interface
<https://github.com/globocom/abyss>`_, which is under development, but is also
a great tool to manage, check logs and monitor applications and services
resources.

Scaling
-------

The Juju_ provisioner allows you to easily add and remove units, enabling one
to scale an application painlessly. It will take care of the application code
replication, and services binding. There's nothing required to the developer to
do in order to scale an application, just add a new unit and Tsuru will do the
trick.

You may also want to scale using the Front end as a Service, powered by `Varnish
<https://www.varnish-cache.org/>`_. One single application might have a whole
farm of Varnish VMs in front of it handling all the traffic.


Built-in Database Services
--------------------------

Tsuru already has a variety of database services available for setup on your
cloud. It allows you to easily create a service instance for your application
usage and bind them together. The service setup for your application is
transparent by the use of environment variables, which are exported in all
instances of the application, allowing your configuration to fit several
environments (like development, staging, production, etc.)


Extensible Service and Platform Support
---------------------------------------

Tsuru allows you to easily add support for new services and new platforms. For
application platforms, it uses `Juju Charms <http://jujucharms.com/>`_, for
services, it has an :doc:`API </services/api>` that it uses to comunicate with
them.

Collaboration
-------------

Manage sharing and deployment of your application. Tsuru uses teams to control
access to resources. A developer may create a team, grant/revoke app access
to/from a team or add/remove new users to/from a team. One can be a member of
multiple teams and control which applications each team has access to.

Easy Server Deployment
----------------------

Tsuru itself is really easy to deploy and manage, you can get it done by
following `these simple steps <http://docs.tsuru.io/en/latest/build.html>`_.

Distributed and Extensible
--------------------------

Tsuru server is easily extensible, distributed and customizable. It has the
concept of ``Provisioner``: a provisioner is a component that takes care of the
orchestration (VM/container management) and provisioning. By default, it will
deploy applications using the Juju_ provisioner, but you can easily implement
your own provisioner and use whatever backend you wish.

When you extend Tsuru, you are able to pratically build a new PaaS in terms of
behavior of provision and orchestration, making use of the great Tsuru
structure. You change the whole Tsuru workflow by implementing a new
provisioner.

.. _Juju: https://juju.ubuntu.com/

Dev/Ops Perspective
-------------------

Tsuru's components are distributed, it is composed by many pieces of software,
each one made to be easily deployable and maintenable. #TODO link architecture overview.

Application Developer Perspective
---------------------------------

We aim to make developers life easier. #TODO link development workflow.
