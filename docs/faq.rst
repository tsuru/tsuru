.. Copyright 2014 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

tsuru Frequently Asked Questions
--------------------------------

* `What is tsuru?`_
* `What is an application?`_
* `What is a unit?`_
* `What is a platform?`_
* `What is a service?`_
* `How does environment variables work?`_
* `How does the quota system works?`_
* `How routing works?`_
* `How are Git repositories managed?`_

This document is an attempt to explain concepts you'll face when deploying and
managing applications using tsuru.  To request additional explanations you can
open an issue on our issue tracker, talk to us at #tsuru @ freenode.net or open
a thread on our mailing list.

What is tsuru?
==============

tsuru is an open source polyglot cloud application platform (PaaS). With tsuru,
you don’t need to think about servers at all.  You can write apps in the
programming language of your choice, back it with add-on resources such as SQL
and NoSQL databases, memcached, redis, and many others. You manage your app
using the tsuru command-line tool and you deploy code using the Git revision
control system, all running on the tsuru infrastructure.

What is an application?
=======================

An application, in tsuru, is a program's source code, dependencies list - on
operational system and language level - and a Procfile with instructions on how
to run that program.  An application has a name, a unique address, a Platform,
associated development teams, a repository and a set of units.

What is a unit?
===============

A unit is an isolated Unix container or a virtual machine - depending on the
configured provisioner. A unit has everything an application needs to run, the
fetched operational system and language level dependencies, the application's
source code, the language runtime, and the applications processes defined on
the Procfile.

What is a platform?
===================

A platform is a well defined pack with installed dependencies for a language or
framework that a group of applications will need. A platform might be a
container template, or a virtual machine image.

For instance, tsuru has a container image for python applications, with
virtualenv installed and other required things needed for tsuru to deploy
applications on top of that platform. Platforms are easily extendable in
tsuru, but currently not managed by it, all tsuru does (by now) is to keep
database records for each existent platform. Every application runs on top of
a platform.

What is a service?
==================

A service is a well defined API that tsuru communicates with to provide extra
functionality for applications. Examples of services are MySQL, Redis, MongoDB,
etc. tsuru has built-in services, but it is easy to create and add new services
to tsuru. Services aren't managed by tsuru, but by its creators.

Check the :doc:`service usage documentation </apps/client/services>` for more
on using services and the :doc:`building your own service tutorial
</services/build>` for a quick start on how to extend tsuru by creating new
services.

How does environment variables work?
====================================

All configurations in tsuru are handled by the use of environment variables. If
you need to connect with a third party service, e.g. twitter's API,
you are probably going to need some extra configurations, like client_id. In
tsuru, you can export those as environment variables, visible only
by your application's processes.

When you bind your application into a service, most likely you'll need to
communicate with that service in some way. Services can export environment
variables by telling tsuru what they need, so whenever you bind your
application with a service, its API can return environment variables for tsuru
to export on your application's units.

How does the quota system works?
================================

Quotas are handled per application and user. Every user has a quota number for
applications. For example, users may have a default quota of 2 applications, so
whenever a user tries to create more than two applications, he/she will receive
a quota exceeded error. There are also per applications quota. This one limits
the maximum number of units that an application may have.

How routing works?
==================

tsuru has a router interface, which makes extremely easy to change the way
routing works with any provisioner. There are two ready-to-go routers: one
using `hipache <https://github.com/dotcloud/hipache>`_ and another with `elb
<http://http://aws.amazon.com/elasticloadbalancing/>`_.

How are Git repositories managed?
=================================

tsuru uses `Gandalf <https://github.com/tsuru/gandalf>`_ to manage git
repositories. Every time you create an application, tsuru will ask Gandalf to
create a related git bare repository for you to push in.

This is the remote tsuru gives you when you create a new app. Everytime you
perform a git push, Gandalf intercepts it, check if you have the required
authorization to write into the application's repository, and then lets the
push proceeds or returns an error message.

Client installation fails with "undefined: bufio.Scanner". What does it mean?
=============================================================================

tsuru clients require Go 1.1 or later. The message ``undefined: bufio.Scanner``
means that you're using an old version of Go. You'll have to `install
<http://golang.org/doc/install>`_ the last version.

If you're using Homebrew on Mac OS, just run:

.. highlight:: bash

::

    $ brew update
    $ brew upgrade go
