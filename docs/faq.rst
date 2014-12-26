.. Copyright 2014 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

Frequently Asked Questions
--------------------------

* `How do environment variables work?`_
* `How does the quota system work?`_
* `How does routing work?`_
* `How are Git repositories managed?`_
* `Client installation fails with "undefined: bufio.Scanner". What does it mean?`_

This document is an attempt to explain concepts you'll face when deploying and
managing applications using tsuru.  To request additional explanations you can
open an issue on our issue tracker, talk to us at #tsuru @ freenode.net or open
a thread on our mailing list.

How do environment variables work?
==================================

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

How does the quota system work?
===============================

Quotas are handled per application and user. Every user has a quota number for
applications. For example, users may have a default quota of 2 applications, so
whenever a user tries to create more than two applications, he/she will receive
a quota exceeded error. There are also per applications quota. This one limits
the maximum number of units that an application may have.

How does routing work?
======================

tsuru has a router interface, which makes it extremely easy to change the way
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
