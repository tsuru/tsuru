.. Copyright 2014 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

++++++++++++++++++++++++++++++
Managing users and permissions
++++++++++++++++++++++++++++++

Starting with tsuru 0.13.0 a new mechanism for managing users and permissions
was introduced. This new mechanism allows for fine-grained control on which
actions are available for each user. While at the same time trying to allow
broad permissions avoiding the need for interaction every time a new permission
is available.

To achieve this goal some concepts will be explained below.

Concepts
--------

Permissions
===========

tsuru includes a fixed number of permissions that may change on each release.
To list all available permissions the command ``tsuru permission-list`` should
be used.

Permissions in tsuru work in a hierarchical fashion and are typically
represented using a dot notation. Granting access to a top-level permission
imply access to all permissions below it.

As an example, consider the following permissions:

* ``app.update.env.set``
* ``app.update.env.unset``
* ``app.deploy``

If a user have access only to ``app.update.env.set`` only this specific action
is available to them. However, it's also possible to grant access to the broader
``app.update`` permission which will allow users to both set and unset
environment variables, but not deploy the applications. If we want to allow a
user to execute all actions related to an application, the even broader
permission ``app`` can be used.

Contexts
========

When applying permissions to a user one do so in regard to a context. Each
permission declares which contexts can be used and it's possible see the
available contexts using the command ``tsuru permission-list``. When a
permission is assigned to a user it needs a context and a value for the chosen
context. Examples of available contexts are:

* ``team``
* ``app``
* ``global``

If a user have the ``app.deploy`` permission for the ``team`` named ``myteam``
it means that they can only deploy applications which ``myteam`` has access. The
same way, it's possible to assign the same ``app.deploy`` permission to a user
with the context ``app`` for one application named ``myappname``. This means the
user can now deploy this specific application called ``myappname``.

The ``global`` context is a special case. It's available to all permissions and
means that the permission always applies. In the previous scenario, if a user
have the ``app.deploy`` permission with a ``global`` context it means that they
can deploy **any** application.

Roles
-----

To better manage permissions it's not possible to directly assign permissions to
users. First you have to create a role including wanted permissions and then
apply this role in regard to a context value to one or more users.

The following commands are available to manage roles and permissions and assign
them to users:

* ``tsuru permission-list``
* ``tsuru role-add``
* ``tsuru role-remove``
* ``tsuru role-list``
* ``tsuru role-permission-add``
* ``tsuru role-permission-remove``
* ``tsuru role-assign``
* ``tsuru role-dissociate``
* ``tsuru role-info``

More details about each command can be found in the :doc:`client documentation
</reference/tsuru-client>`.

An example of the typical scenario for adding a new role and assigning it to a
user is the following:

.. highlight:: bash

::

    $ tsuru role-add app_reader_restarter team
    Role successfully created!
    $ tsuru role-list
    +----------------------+---------+-------------+
    | Role                 | Context | Permissions |
    +----------------------+---------+-------------+
    | AllowAll             | global  | *           |
    +----------------------+---------+-------------+
    | app_reader_restarter | team    |             |
    +----------------------+---------+-------------+
    $ tsuru role-permission-add app_reader_restarter app.read app.update.restart
    Permission successfully added!
    $ tsuru role-list
    +----------------------+---------+--------------------+
    | Role                 | Context | Permissions        |
    +----------------------+---------+--------------------+
    | AllowAll             | global  | *                  |
    +----------------------+---------+--------------------+
    | app_reader_restarter | team    | app.read           |
    |                      |         | app.update.restart |
    +----------------------+---------+--------------------+
    $ tsuru user-list
    +-------------------+------------------+-------------+
    | User              | Roles            | Permissions |
    +-------------------+------------------+-------------+
    | admin@example.com | AllowAll(global) | *(global)   |
    +-------------------+------------------+-------------+
    | myuser@corp.com   |                  |             |
    +-------------------+------------------+-------------+
    $ tsuru role-assign app_reader_restarter myuser@corp.com myteamname
    Role successfully assigned!
    $ tsuru user-list
    +-------------------+---------------------------------------+-------------------------------------+
    | User              | Roles                                 | Permissions                         |
    +-------------------+---------------------------------------+-------------------------------------+
    | admin@example.com | AllowAll(global)                      | *(global)                           |
    +-------------------+---------------------------------------+-------------------------------------+
    | myuser@corp.com   | app_reader_restarter(team myteamname) | app.read(team myteamname)           |
    |                   |                                       | app.update.restart(team myteamname) |
    +-------------------+---------------------------------------+-------------------------------------+


From this moment the user named ``myuser@corp.com`` can read and restart all
applications belonging to the team named ``myteamname``.

Default roles
=============

It's possible to have default roles that are applied to a user when some event
happens on tsuru. Example of such events are ``user-create`` and
``team-create``. A list of all possible events can be found running the command
``tsuru role-default-list``. Commands ``tsuru role-default-add`` and ``tsuru
role-default-remove`` should be used to include or remove new roles in an event.

A common use for default roles would be replicating the behavior of tsuru on
versions prior to 0.13.0. A new user would always be allowed to create a new
team and would also be allowed to create new applications on the newly created
team.

To achieve this with default roles first two roles need to be created, let's
call them ``team-creator`` and ``team-member``. ``team-creator`` would use the
``global`` context and include the ``team.create`` permission. ``team-member``
would use the ``team`` context and include the ``app`` permission.

With these roles created we only need to add them as default on the appropriate
event:

.. highlight:: bash

::

    $ tsuru role-default-add --user-create team-creator --team-create team-member


.. _migrating_perms:

Adding members to a team
========================

When managing teams, It's very common to add new members to a team or add members
to a new team. To do this on Tsuru you'll need to use ``role-assign`` command, as
follows:

.. highlight:: bash

::

    $ tsuru role-assign <role> <user@email.com> <team>

Migrating
---------

When you already have an existing tsuru installation it will be necessary to
create roles and assign them to all existing users, otherwise they will no
longer be able to execute any action in tsuru.

To make this process easier we created a migration to help with the transition.
The goal of this migration is to roughly give all existing users the same set of
permissions they already had on tsuru. To accomplish this it'll create 3
different roles: ``admin``, ``team-member`` and ``team-creator``.

The ``admin`` role will have a global context for the root permission and will
be assigned to all users that are members to the ``admin-team`` described in
``tsuru.conf`` file. This users will be able to do anything, anywhere.

The ``team-member`` role will have a ``team`` context and the following
permissions:

* ``app``
* ``team``
* ``service-instance``

And will be assigned to all users for each team name the user is a member of.

The ``team-creator`` role will only include the ``team.create`` permission with
a ``global`` context and will also be assigned to all users.

Also the role ``team-creator`` will be assigned as a default role when a new
user is created. And the ``team-member`` role will be the default role assigned
to a user when they create a new team.

Running this migration is optional. If you choose execute it simply run:

.. highlight:: bash

::

    $ tsurud [--config <path to tsuru.conf>] migrate --name migrate-roles


Bootstrapping
-------------

For a new tsuru installation the first user created should have a role with a
root permission. To create this user a new command was created in the tsuru
daemon application (``tsurud``) and should be executed right after its
installation:

.. highlight:: bash

::

    $ tsurud [--config <path to tsuru.conf>] root-user-create myemail@somewhere.com
    # type a password and confirmation (only if using native auth scheme)
