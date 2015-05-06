.. Copyright 2014 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

============
Architecture
============

API
---

The API component (also called `tsr`) is a RESTful API server written with
``Go``. The API is responsible for the deploy workflow and the lifecycle of
applications.

Command-line clients interact with this component.

Database
--------

The database component is a `MongoDB` server.

Queue/Cache
-----------

The queue and cache component uses `Redis`.

Gandalf
-------

`Gandalf` is a REST API to manage Git repositories and users and provides
access to them over SSH.

Registry
--------

The registry component hosts `Docker` images.

Router
------

The router component routes traffic to application units (Docker containers).
