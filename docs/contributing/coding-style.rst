.. Copyright 2015 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

++++++++++++
Coding style
++++++++++++

Please follow these coding standards when writing code for inclusion in tsuru.

Formatting
==========

* Follow the `go formatting style <http://golang.org/doc/effective_go.html#formatting>`_

Naming standards
================

New<Something>
--------------

is used by the <Something> `constructor`:

::

    NewApp(name string) (*App, error)

Add<Something>
--------------

is a `method` of a type that has a collection of <Something>'s. Should receive an instance of <Something>:

::

    func (a *App) AddUnit(u *Unit) error

Add
---

is a collection `method` that adds one or more elements:

::

    func (a *AppList) Add( apps ...*App) error

Create<Something>
-----------------

it's a `function` that saves an instance of <Something>
in the database. Should receive an instance of <Something>.

::

    func CreateApp(a *App) error

Delete<Something>
-----------------

it's a `function` that deletes an instance of <Something> from database.

Remove<Something>
-----------------

it's opposite of Add<Something>.
