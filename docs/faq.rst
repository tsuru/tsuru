.. Copyright 2013 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

Tsuru Frequently Asked Questions
--------------------------------

* `What is Tsuru?`_
* `What is an application?`_
* `What is a unit?`_
* `What is a platform?`_
* `What is a service?`_
* `How does environment variables work?`_
* `How does the quota system works?`_
* `How routing works?`_
* `How are repositories managed?`_

This document is an attempt to explain concepts you'll face when deploying and managing applications using Tsuru.
To request additional explanations you can open an issue on our issue tracker, talk to us at #tsuru @ freenode.net
or open a thread on our mailing list.

What is Tsuru?
==============

Tsuru is an open source polyglot cloud application platform (PaaS). With tsuru, you don’t need to think about servers at all.
You can write apps in the programming language of your choice, back it with add-on resources such as SQL and NoSQL databases,
memcached, redis, and many others. You manage your app using the tsuru command-line tool and you deploy code using the Git
revision control system, all running on the tsuru infrastructure.

What is an application?
=======================

An application, in Tsuru, is a program's source code, dependencies list - on operational system and language level -
and a Procfile with instructions on how to run that program.
An application has a name, a unique address, a Platform, associated development teams, a repository and a set of units.

What is a unit?
===============

A unit is an isolated Unix container or a virtual machine - depending on the configured provisioner. A unit has everything
an application needs to run, the fetched operational system and language level dependencies, the application's source code,
the language runtime, and the applications processes defined on the Procfile.

What is a platform?
===================

A platform is a well defined pack with installed dependencies for a language or framework that a group of applications will need.
A platform might be a container template, or a virtual machine template (golden image). For instance, Tsuru has a container image for python
applications, with virtualenv installed and other required things needed for Tsuru to deploy applications on top of that platform.
Platforms are easily extendable in Tsuru, but currently not managed by it, all Tsuru does (by now) is to keep database records
for each existent platform.
Every application runs on top of a platform, there is no way of deploying an application without an associated platform.

What is a service?
==================

A service is a well defined api that Tsuru communicates with to provide extra functionality for your application. Examples of
services are MySQL, Redis, MongoDB, etc. Tsuru has built-in services, but it is really easy to create and add new services to Tsuru.
Services aren't managed by Tsuru, but by it's creators. Althought you can host your service api on Tsuru as an application, Tsuru does
not give enough flexibility (like sshing the application units, although you can run commands) to properly manage a service. You can do it,
but it's a bit harder.
Check the :doc:`service usage documentation </apps/client/services>` for more on using services and the :doc:`building your own service tutorial </services/build>`
for a quick start on how to extend Tsuru by creating new services.

How does environment variables work?
====================================

All configurations in Tsuru are handled by the use of environment variables. If you need to connect with a third party service, e.g. twitter's api,
you are probably going to need some extra configurations, like client_id. In Tsuru, you can export those as environment variables, visible only
by your application's processes. This is done using a queue.
Whenever you (or a service, we'll get there soon) export environment variables into an application, Tsuru puts your request on the queue, when it reads it,
it writes the variables and its values on a file on the application's unit, called apprc. This file watched by a circus plugin, writen by Tsuru developers.
When circus finds a change in that file, it reloads the application environment variables, adding the recently added envvars. But you don't need to know
that to use environment variables on Tsuru, everything is transparent for the end user, as you'll experience.
When you bind your application into a service, most likely you'll need to communicate with that service in some way. Services can export environment variables
by telling Tsuru what they need, so whenever you bind your application with a service, its api can return environment variables for Tsuru to export on your
application's units.

How does the quota system works?
================================

Quotas are handled per application and user. Every user has a quota number for applications. For example, by default, a user has a quota of 2 applications, so
whenever a user tries to create more than two applications, he/she will receive a quota exceeded error. There are also per applications quota. This one limits
the maximum number of units an application may have.

How routing works?
==================

Tsuru has a router interface, which makes extremely easy to change the way routing works with any provisioner. There are two ready-to-go routers: one using
`hipache <https://github.com/dotcloud/hipache>`_ and another with `nginx <http://wiki.nginx.org/>`_.

How are repositories managed?
=============================

Tsuru uses `Gandalf <https://github.com/globocom/gandalf>`_ to manage git repositories. Every time you create an application
Tsuru will ask Gandalf to create a related git bare repository for you to push in.  This is the remote Tsuru gives you when
you create a new app. Everytime you perform a git push, Gandalf intercepts it, check if you have the required authorization
to write into the application's repository, and then lets the push proceeds or returns an error message.
