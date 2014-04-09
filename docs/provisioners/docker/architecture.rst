Docker Provisioner Architecture
===============================

This document describes how tsuru works when configured with docker provisioner.
`Docker <http://docker.io>`_

The docker provisioner is responsible for provisioning your application units. Everytime your perform an action
in your application tsuru repasses the request with specific parameters to the configured provisioner. In this document
you will learn how the docker provisioner reacts facing those actions.

Given the app creation -> deploy workflow.

App Provisioning
----------------

When you create an application tsuru asks the provisioner to provision the application, the docker provisioner
will do nothing in this action, the only change is that tsuru creates the application on the database. Docker
provisioner will wait until you perform a deploy, so it can create a base image to your application.

Deployment
----------

When you perform a git push into your application repository on tsuru the custom
`pre-receive git hook <http://git-scm.com/book/en/Customizing-Git-Git-Hooks#Server-Side-Hooks>`_ is triggered, this hook will ask tsuru
to deploy your application, tsuru will then repass the action to docker. Docker will run a container, clone your
application code to it and install all dependencies specified by your application, then it will generate an image of that
container and store its id on the database, this container is then destroyed and a new one is run starting your application.
This allows an easy and fast scalability for your application, whenever you need a new unit tsuru can deploy one in a few seconds.

Every deploy will trigger this process, resulting in a new image with the deployed version and new dependencies if any.

HTTP Routing
------------

Because containers are ephemeral their routes changes everytime a deploy is performed. So we need an easy and fast way to manage routes to
containers, by default the docker provisioner uses `Hipache <https://github.com/dotcloud/hipache>`_ router.
Routes to containers are managed transparently by the docker provisioner. The hipache router also acts as a load balancer to the containers,
distributing traffic using a round robin algorithm.
