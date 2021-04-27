.. Copyright 2013 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

Frequently Asked Questions
--------------------------

* `How do environment variables work?`_
* `How does the quota system work?`_
* `How does routing work?`_

This document is an attempt to explain concepts you'll face when deploying and
managing applications using tsuru.  To request additional explanations you can
open an issue on our issue tracker, talk to us at #tsuru @ freenode.net or open
a thread on our mailing list.

How do environment variables work?
==================================

All configurations in tsuru are handled by the use of environment variables. If
you need to connect with a third party service, e.g. twitter's API, you are
probably going to need some extra configurations, like client_id. In tsuru, you
can export those as environment variables, visible only by your application's
processes.

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
using `planb <https://github.com/tsuru/planb>`_ and another with `galeb
<http://galeb.io/>`_.

.. note::

    as of 0.10.0 version **tsuru** supports more than one router. You can have
    a default router, configured by "docker:router" and you can define a custom
    router by plan
