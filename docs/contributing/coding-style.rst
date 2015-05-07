.. Copyright 2015 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

++++++++++++
Coding style
++++++++++++

Please follow these coding standards when writing code for inclusion in tsuru.

Formatting
==========

* Follow the `Go formatting style <http://golang.org/doc/effective_go.html#formatting>`_

Naming standards
================

New<Something>
--------------

is used the constructor of `Something`:

::

    NewApp(name string) (*App, error)

Add<Something>
--------------

is a `method` of a type that has a collection of `Something`s. Should receive an instance of `Something`:

::

    func (a *App) AddUnit(u *Unit) error

Add
---

is a method of a collection that adds one or more elements:

::

    func (a *AppList) Add(apps ...*App) error

Create<Something>
-----------------

is a function that saves an instance of `Something`. Unlike ``NewSomething``,
the create function would create a persistent version of `Someting`. Storing it
in the database, a remote API, the filesystem or wherever `Something` would be
stored "forever".

Comes in two versions:

#. One that receives the instance of `Something` and returns an error:

    ::

        func CreateApp(a *App) error

#. Other that receives the required parameters and returns the an instance of
   `Something` and an error:

    ::

        func CreateUser(email string) (*User, error)

Delete<Something>
-----------------

is a function that destroy an instance of `Something`. Destroying may involve
processes like removing it from the database and some directory in the
filesystem.

For example:

::

    func DeleteApp(app *App) error

Would delete an application from the database, delete the repository, remove
the entry in the router, and anything else that depends on the application.

It's also valid to write the function so it receives some other kind of values
that is able to identify the instance of `Something`:

::

    func DeleteApp(name string) error

Remove<Something>
-----------------

is the opposite of Add<Something>.

Including the package in the name of the function
=================================================

For functions, it's also possible to omit `Something` when the name of the
package represents `Something`. For example, if there's a package named "app",
the function CreateApp could be just "Create". The same applies to other
functions. This way callers won't need to write verbose code like
``something.CreateSomething``, preferring ``something.Create``.
