.. Copyright 2014 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.


+++++++++++++++++++
Creating a platform
+++++++++++++++++++

Overview
========

If you need a platform that's not already available in our `platforms repository
<https://github.com/tsuru/platforms>`_ it's pretty easy to create a new one
based on an existing one.

Platforms are Docker images that are used to deploy your application code on tsuru. tsuru provides a
base image which platform developers can use to build upon: `base-platform <https://github.com/tsuru/base-platform>`_.
This platform provides a base deployment script, which handles package downloading and
extraction in proper path, along with operating system package management.

Every platform in the `repository <https://github.com/tsuru/platforms>`_ extends the base-platform
adding the specifics of each platform. They are a great way to learn how to create a new one.

An example
==========

Let's supose we wanted to create a nodejs platform. First, let's define it's Dockerfile:

.. highlight:: bash

::

    FROM	tsuru/base-platform
    ADD	. /var/lib/tsuru/nodejs
    RUN	cp /var/lib/tsuru/nodejs/deploy /var/lib/tsuru
    RUN	/var/lib/tsuru/nodejs/install

In this file, we are extending the ``tsuru/base-platform``, adding our deploy and install
scripts to the right place: ``/var/lib/tsuru``.

The install script runs when we add or update the platform on tsuru. It's the perfect place
to install dependencies that every application on it's platform needs:

.. highlight:: bash

::

    #!/bin/bash -le

    SOURCE_DIR=/var/lib/tsuru
    source ${SOURCE_DIR}/base/rc/config

    apt-get update
    apt-get install git -y
    git clone https://github.com/creationix/nvm.git /etc/nvm
    cd /etc/nvm && git checkout `git describe --abbrev=0 --tags`

    cat >> ${HOME}/.profile <<EOF
    if [ -e ${HOME}/.nvm_bin ]; then
    	export PATH="${HOME}/.nvm_bin:$PATH"
    fi
    EOF

As it can be seen, we are just installing some dependencies and preparing the environment for our applications.
The ``${SOURCE_DIR}/base/rc/config`` provides some bootstrap configuration that are usually needed.

Now, let's define our deploy script, which runs every time a deploy occurs:

.. highlight:: bash

::

    #!/bin/bash -le

    SOURCE_DIR=/var/lib/tsuru
    source ${SOURCE_DIR}/base/rc/config
    source ${SOURCE_DIR}/base/deploy

    export NVM_DIR=${HOME}/.nvm
    [ ! -e ${NVM_DIR} ] && mkdir -p ${NVM_DIR}

    . /etc/nvm/nvm.sh

    nvm install stable

    rm -f ~/.nvm_bin
    ln -s $NVM_BIN ~/.nvm_bin

    if [ -f ${CURRENT_DIR}/package.json ]; then
    	pushd $CURRENT_DIR && npm install --production
    	popd
    fi

Once again we run some base scripts to do some heavy lifting: ``${SOURCE_DIR}/base/rc/config`` and
``${SOURCE_DIR}/base/deploy``. After that, it's just a matter of application specifics dependencies using
npm.

Now, we can move on and add our newly created platform.

Adding your platform to tsuru
=============================

After creating you platform as a Dockerfile, you can add it to tsuru using the client:

.. highlight:: bash

::

    $ tsuru platform add your-platform-name --dockerfile http://url-to-dockerfile

If you push your image to an Docker Registry, you can use:

.. highlight:: bash

::

    $ tsuru platform add your-platform-name -i your-user/image-name
