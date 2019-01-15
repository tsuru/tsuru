.. Copyright 2016 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

+++++++++++++++++++++++++++++++++++
Deploying Docker Image applications
+++++++++++++++++++++++++++++++++++

Overview
========

This document is a hands-on guide to deploy a simple Docker Image web application.

Creating the app
================

To create an app, you need to use the command `app-create`:

.. highlight:: bash

::

    $ tsuru app-create <app-name> <app-platform>

For Docker Images, doesn't exist a specific platform, but we can use ``static``! Let's be over creative and develop a hello
world tutorial-app, let's call it "helloworld":

.. highlight:: bash

::

    $ tsuru app-create helloworld static

To list all available platforms, use the command `platform-list`.

You can see all your applications using the command  `app-list`:

.. highlight:: bash

::

    $ tsuru app-list
    +-------------+-------------------------+--------------------------------+
    | Application | Units State Summary     | Address                        |
    +-------------+-------------------------+--------------------------------+
    | helloworld  | 0 of 0 units in-service | helloworld.192.168.50.4.nip.io |
    +-------------+-------------------------+--------------------------------+

Application code
================

A simple `Dockerfile`:

.. highlight:: Dockerfile

::

    FROM golang
    RUN mkdir /app
    WORKDIR /app
    ADD . /app/
    RUN go build .
    ENTRYPOINT ./app
    
.. note::
    Notice that you do not have to EXPOSE a port in your Dockerfile.
    When a EXPOSE clause is used, the port mapping goes to the exposed port.
    If you do not expose a port, the port is mapped to the $PORT environment variable.

A simple web application in Go `main.go`:

.. highlight:: go

::

    package main

    import (
        "fmt"
        "net/http"
        "os"
    )

    func main() {
        c := make(chan os.Signal, 1)
        signal.Notify(c, os.Interrupt)
        go func(){
            for sig := range c {
                if sig == os.Interrupt || sig == os.Kill {
                    os.Exit(1)
                }
            }
        }()
        http.HandleFunc("/", hello)
        fmt.Println("running on "+os.Getenv("PORT"))
        http.ListenAndServe(":"+os.Getenv("PORT"), nil)
    }

    func hello(res http.ResponseWriter, req *http.Request) {
        fmt.Fprintln(res, "hello, world!")
    }
    
.. note::
    Make sure that the app listens on the port provided by the $PORT environment variable


Building the image
==================

.. highlight:: bash

::

    docker login registry.myserver.com

    docker build -t registry.myserver.com/image-name .


Don't forget the dot(.) at the end of the command, this indicates where the Dockerfile is placed

Sending the image to registry
=============================

.. highlight:: bash

::

    docker push registry.myserver.com/image-name


Docker Image deployment
=======================

After pushing your image to your Docker image registry, you can do the deploy using the command `tsuru app-deploy -i`.

.. highlight:: bash

::

    tsuru app-deploy -i registry.myserver.com/image-name -a helloworld


.. note::

    This image should be in a registry and be accessible by the nodes.
    Image should also have a Entrypoint or a Procfile at given paths, / or /app/user/ or /home/application/current
    The Image should not expose a Port! This is done automatically using the $PORT environment variable.


Running the application
=======================

Now that the app is deployed, you can access it from your browser, getting the
IP or host listed in ``app-list`` and opening it. For example,
in the list below:

::

    $ tsuru app-list
    +-------------+-------------------------+--------------------------------+
    | Application | Units State Summary     | Address                        |
    +-------------+-------------------------+--------------------------------+
    | helloworld  | 1 of 1 units in-service | helloworld.192.168.50.4.nip.io |
    +-------------+-------------------------+--------------------------------+

It's done! Now we have a simple Docker image project deployed on tsuru.

Now we can access your app in the URL displayed in `app-list`
("helloworld.192.168.50.4.nip.io" in this case).

Going further
=============

For more information, you can dig into `tsuru docs <http://docs.tsuru.io>`_, or
read `complete instructions of use for the tsuru command
<https://tsuru-client.readthedocs.org>`_.
