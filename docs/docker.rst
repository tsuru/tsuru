.. Copyright 2013 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

+++++++++++++++++++++++++++++++++++++++++
Build your own PaaS with tsuru and Docker
+++++++++++++++++++++++++++++++++++++++++

TODO

Under the hood
--------------

Before you dive in, check out the scripts used to build the
`base image <https://github.com/flaviamissi/basebuilder>` tsuru uses for docker.
TODO: push the image to docker registry (tsuru/base).
The docker provisioner is quite simple. The main magic is on the deploy step.
It is composed of four steps:

    - Deploy and dependencies installation: this step runs the deploy script
      from the docker image, it receives the application's repository url
    - Commits a new docker image: when done with the previous step, tsuru
      will generate an image with the deployed code and the required
      dependencies and store its id.
    - Remove the first container: since we have generate an image from it,
      there's no need to let trash away.
    - Run a container: using the generated docker image, tsuru spawns a
      container, running circusd. After this step, your application is ready
      to receive requests.
