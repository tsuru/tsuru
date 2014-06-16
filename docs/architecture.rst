.. Copyright 2014 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

++++++++++++++++++
tsuru architecture
++++++++++++++++++

api
===

The api is the heart of `tsuru`. The api is responsible to the deploy workflow
and the lifecycle of `apps`.

provisioners
------------

The provisioner is responsible for provision the `units`.

There is only one supported provisioner right now:

* docker

routers
-------

The router routes incoming traffic to the application units.

Currently, there is two routers:

* elb
* hipache

collector
=========

The `collector` is a loop process that checks and updates the unit states. 

mongodb
=======

tsuru uses `mongodb` to store all data about `apps`, `units`, `services`, `users` and teams.

gandalf
=======

`gandalf` is a REST api to manage git repositories, users and provide access to them over SSH.
