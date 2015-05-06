.. Copyright 2015 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

tsuru client plugins
====================

Installing a plugin
-------------------

Let's install a plugin. There are two ways to install a plugin. The first way
is to move your plugin to ``$HOME/.tsuru/plugins``. The other way is to use the command
``tsuru plugin-install``.

``tsuru plugin-install`` will download the plugin file to
``$HOME/.tsuru/plugins``. The syntax for this command is:

.. highlight:: bash

::

    $ tsuru plugin-install <plugin-name> <plugin-url>

Listing installed plugins
-------------------------

To list all installed plugins, users can use the command ``tsuru plugin-list``:

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

To remove a plugin just use the command ``tsuru plugin-remove`` passing the
name of the plugin as argument:

.. highlight:: bash

::

    $ tsuru plugin-remove <plugin-name>
    Plugin "<plugin-name>" successfully removed!

Creating your own plugin
------------------------

All you need to do is to create a new file that can be executed. You can use
Shell Script, Python, Ruby, etc.

As an example, we're going to show how to create a Hello world plugin, that
just prints "hello world!" in the screen. Let's use Shell Script in this
plugin:

.. highlight:: bash

::

    #!/bin/bash -e
    echo "hello world!"

You can use the gist (https://gist.github.com) as host for your plugin, and run
``tsuru plugin-install`` to install it:

.. highlight:: bash

::

    $ tsuru plugin-install hello https://gist.githubusercontent.com/fsouza/702a767f48b0ceaafebe/raw/9bcdf9c015fda5ca410ca5eaf254a806bddfcab3/hello.bash
