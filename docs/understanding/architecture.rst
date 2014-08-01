.. Copyright 2014 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

============
Architecture
============

API
---

API component is a RESTful API server written with `Go`.
The API is responsible to the deploy workflow and lifecycle
of `apps`.

Command-line clients interact with this component.

Database
--------

The database component is a `MongoDB` server.


Queue/Cache
-----------

The queue and cache component uses `Redis`.


Gandalf
-------

`Gandalf` is a REST api to manage git repositories, users and provide access
to them over SSH.

Registry
--------

The registry component hosts `Docker`_ images.

Router
------

The router component routes traffic to application units.
