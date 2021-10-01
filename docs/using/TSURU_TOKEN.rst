.. Copyright 2021 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

Automatic Authentication Through Token
===============

Tsuru's client terminal enables automatic authentication through tokens. 

To create a user token, create a user:

.. highlight:: bash

::

    $ tsuru user-create <email>


Login:

.. highlight:: bash

::

    $ tsuru login [email]

You'll be asked to provide e-mail and password for authentication. After that, the generated token by the tsuru server will be stored in 

.. highlight:: bash

::

    $ {HOME}/.tsuru/token
    
Now you won't be asked for e-mail and password to login again but be aware that some actions will require authentication.
