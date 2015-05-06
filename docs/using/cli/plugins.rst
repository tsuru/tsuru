.. Copyright 2015 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

Guide to create tsuru cli plugins
=================================

Installing a plugin
-------------------

Let's install a plugin. There are two ways to install.  The first way is to
move your plugin to ``$HOME/.tsuru/plugins``.  The other way is to use ``tsuru
plugin-install`` command.

``tsuru plugin-install`` will download the plugin file to
``$HOME/.tsuru/plugins``.  The syntax for this command is:

.. highlight:: bash

::

    $ tsuru plugin-install <plugin-name> <plugin-url>

Listing installed plugins
-------------------------

To list all installed plugins, users can use the ``tsuru plugin-list`` command:

.. highlight:: bash

::

    $ tsuru plugin-list
    plugin1
    plugin2

Executing a plugin
------------------

To execute a plugin just follow this pattern ``tsuru <plugin-name> <args>``:

.. highlight:: bash

::

    $ tsuru <plugin-name>
    <plugin-output>

Removing a plugin
-----------------

To remove a plugin just use the ``tsuru plugin-remove`` command passing the
name of the plugin as argument:

.. highlight:: bash

::

    $ tsuru plugin-remove <plugin-name>
    Plugin "<plugin-name>" successfully removed!

Creating your own plugin
------------------------

Everything you need to do is to create a new file that can be executed. You can
use Bash, Python, Ruby, eg.

Let's create a Hello world plugin that prints "hello world" as output.  Let's
use ``bash`` to write our new plugin.

.. highlight:: bash

::

    #! /bin/bash
    echo "hello world!"

You can use the gist (https://gist.github.com) as host for your plugin, and run
``tsuru plugin-install`` to install it.
