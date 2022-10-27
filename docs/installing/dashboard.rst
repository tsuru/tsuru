.. Copyright 2016 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

+++++++++
Dashboard
+++++++++

One of the ways to interact with your tsuru installation is using the
`tsuru dashboard <https://github.com/tsuru/tsuru-dashboard>`_.
The dashboard provides interesting features for both tsuru users (application information,
metrics and logs for example) and tsuru admins (hosts metrics, healings and much more).

The dashboard runs as a regular tsuru Python application. This guide will cover:

    1. Adding the Python platform
    2. Creating the dashboard app
    3. Deploying the tsuru dashboard

You should already have a pool and at least one docker node to run your applications.
Please refer to :doc:`adding nodes </installing/adding-nodes>` for more details.

--------------------------
Adding the Python platform
--------------------------

Platforms are responsible for building and running your application. The dashboard requires
the Python platform, which can be easily installed with:

.. highlight:: bash

::

    tsuru platform add python

This will install the default Python platform. Please refer to :doc:`add platform </managing/add-platform>`
for more details.

--------------------------
Creating the dashboard app
--------------------------

Now, lets create the dashboard application:

.. highlight:: bash

::

    tsuru app create tsuru-dashboard python --team admin

This will create an application called tsuru-dashboard which uses the Python platform
and belongs to the admin team. Please refer to the
`app create client reference <https://tsuru-client.readthedocs.io/en/latest/reference.html#create-an-application>`_
for more information.


-----------------------
Deploying the dashboard
-----------------------

There are several ways to deploy an application in tsuru: app deploy and
app deploy using docker images. The easiest way to deploy the dashboard is by using
app deploy with its docker image. To do that, simply type:

.. highlight:: bash

::

    tsuru app deploy -a tsuru-dashboard -i tsuru/dashboard

This will deploy the docker image `tsuru/dashboard <https://hub.docker.com/r/tsuru/dashboard/>`_
to the app we just created. Please refer to the
`app deploy client reference <https://tsuru-client.readthedocs.io/en/latest/reference.html#deploy>`_
for more information.

Once the deploy finishes, we can run:

.. highlight:: bash

::

    tsuru app info -a tsuru-dashboard


to check it's address and access it on our browser.
