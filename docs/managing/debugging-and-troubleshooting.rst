.. Copyright 2016 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

+++++++++++++++++++++++++++++++
Debugging and Troubleshooting
+++++++++++++++++++++++++++++++

Overview
========

When tsuru API is running slow or hanging, we may want troubleshoot it to
discover what is the source of the problem.

One of the ways to debug/troubleshoot the tsuru API is by analyzing the
running goroutines.

We may do it by cURL or by sending a USR1 signal.

Using cURL
-------------

Tsuru has a path that can be used by cURL to return all the goroutines in
execution. This path is : /debug/goroutines

.. highlight:: bash

::

    $ curl -X GET -H "Authorization: bearer <API key>" <tsuru-host>:<port>/debug/goroutines


Using SIGUSR1
---------------

If for some reason the process is no longer accepting connections, the solution
using cURL will not work.

Alternatively, tsuru API is able to handle the USR1 signal to dump goroutines
in the tsurud execution screen:

.. highlight:: bash

::

    $ kill -s USR1 <tsurud-PID>
