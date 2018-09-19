.. Copyright 2018 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

Handling tokens
===============

Every action in tsuru requires a token. If you need to do some kind of
automation, instead of setting a user token, you can create a team token.

To create a team token, use the `token create` command:

.. highlight:: bash

::

    $ tsuru token create --id my-ci-token --team myteam \
        --description "CI token" --expires 48h
    Token "my-ci-token" created: b3bc4ded93dd9a799874b564835d362aa1274be0e9511f29d3f78dc8517af176

The `expires` flag is optional. By default, team tokens don't expire.

Now you can set new permissions to this token with `role assign` command:

.. highlight:: bash

::

    $ tsuru role assign deployer my-ci-token
    Role successfully assigned!

This example assumes a role called `deployer` was previously created. A user
can only add permissions that he owns himself.

To list all team tokens you have permission to see, use `token list` command:

.. highlight:: bash

::

    $ tsuru token list
    +-------------+--------+-------------+-------------------------+----------------------------------+----------------------------------------------------+-----------------+
    | Token ID    | Team   | Description | Creator                 | Timestamps                       | Value                                              | Roles      |
    +-------------+--------+-------------+-------------------------+----------------------------------+----------------------------------------------------+-----------------+
    | my-ci-token | myteam | CI token    | me@example.com          |  Created At: 19 Sep 18 11:42 -03 | b3bc4ded93dd9a799874b564835d362aa1274be0e9511f29d↵ | deployer() |
    |             |        |             |                         |  Expires At: -                   | 3f78dc8517af176                                    |          |
    |             |        |             |                         | Last Access: -                   |                                                    |          |
    +-------------+--------+-------------+-------------------------+----------------------------------+----------------------------------------------------+-----------------+

Now you can use the token in `Value` column above to make deploys to apps
owned by `myteam` team.
