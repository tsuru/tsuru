.. Copyright 2015 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

++++++++++++++++++++++++++++++++++++++
Managing Git repositories and SSH keys
++++++++++++++++++++++++++++++++++++++

There are two deployment flavors in tsuru: using ``git push`` and ``tsuru
app-deploy``. The former is optional, while the latter will always be
available. This document focus on the usage of the Git deployment flavor.

In order to allow tsuru users to use ``git push`` for deployments, tsuru
administrators need to :doc:`install and configure Gandalf
</installing/gandalf>`.

Gandalf will store and manage all Git repositories and SSH keys, as well as
users. When tsuru is configured to use Gandalf, it will interact with the
Gandalf API in the following actions:

* When creating a new user in tsuru, a corresponding user will be created in
  Gandalf;
* When removing a user from tsuru, the corresponding user will be removed from
  Gandalf;
* When creating an app in tsuru, a new repository for the app will be created
  in Gandalf. All users in the team that owns the app will be authorized to
  access this repository;
* When removing an app, the corresponding repository will be removed from
  Gandalf;
* When adding a user to a team in tsuru, the corresponding user in Gandalf will
  gain access to all repositories matching the applications that the team has
  access to;
* When removing a user from a team in tsuru, the corresponding user in Gandalf
  will lose access to the repositories that he/she has access to because of the
  team he/she is leaving;
* When adding a team to an application in tsuru, all users from the team will
  gain access to the repository matching the app;
* When removing a team from an application in tsuru, all users from the team
  will lose access to the repository, unless they're in another team that also
  have access to the application.

When user runs a ``git push``, the communication happens directly between the
user host and the Gandalf host, and Gandalf will notify tsuru the new
deployment using a git hook.

Managing SSH public keys
========================

In order to be able to send git pushes to the Git server, users need to have
their key registered in Gandalf. When Gandalf is enabled, tsuru will enable
the usage of three commands for SSH public keys management:

* tsuru key-add
* tsuru key-remove
* tsuru key-list

Each of these commands have a corresponding API endpoint, so other clients of
tsuru can also manage keys through the API.

tsuru will not store any public key data, all the data related to SSH keys is
handled by Gandalf alone, and when Gandalf is not enabled, those key commands
will not work.

Adding Gandalf to an already existing tsuru cluster
===================================================

In the case of an old tsuru cluster running without Gandalf, users and
applications registered in tsuru won't be available in the newly created
Gandalf server, or both servers may be out-of-sync.

When Gandalf is enabled, The tsuru-server daemon (tsr) will synchronize
automatically all users and applications on start-up. While this slows down the
first start-up after enabling Gandalf, it means that the administrator of tsuru
do not need to run any synchronization or migration script: just plug Gandalf
to tsuru and tsuru will handle all the data.
