.. Copyright 2015 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

++++++++++++
Adding Nodes
++++++++++++

Nodes are physical or virtual machines with a Docker installation.

Nodes can be either created manually, by provisioning a machine and installing
Docker on it, in which case they have to be registered in tsuru. Or they can be
automatically managed by tsuru, which will handle machine provisioning and Docker
installation using your :ref:`IaaS configuration <iaas_configuration>`.

The automatically managed option is preferred starting with tsuru 0.6.0. There are
advantages like automatically healing and scaling of Nodes which will be
implemented in the future.

The sections below describe how to add managed nodes and manually created nodes
respectively.

Managed nodes
=============

First step is configuring your IaaS provider in your tsuru.conf file. Please see
the details in :ref:`IaaS configuration <iaas_configuration>`

Assuming you're using EC2, this will be something like:

.. highlight:: yaml

::

  iaas:
    default: ec2
    node-protocol: http
    node-port: 2375
    ec2:
      key-id: xxxxxxxxxxx
      secret-key: yyyyyyyyyyyyy

After you have everything configured, adding a new docker node is done by
calling `docker-node-add <http://tsuru-admin.readthedocs.org/en/latest/#docker-node-add>`_ in
:doc:`tsuru-admin </reference/tsuru-admin>` command. This command will receive
a map of key=value params which are IaaS dependent. A list of possible key
params can be seen calling:

.. highlight:: bash

::

    $ tsuru-admin docker-node-add iaas=ec2

    EC2 IaaS required params:
      image=<image id>         Image AMI ID
      type=<instance type>     Your template uuid

    Optional params:
      region=<region>          Chosen region, defaults to us-east-1
      securityGroup=<group>    Chosen security group
      keyName=<key name>       Key name for machine


Every key=value pair will be added as a metadata to the Node and you can send
After registering your node, you can list it calling `tsuru-admin docker-node-list <http://tsuru-admin.readthedocs.org/en/latest/#docker-node-list>`_

.. highlight:: bash

::

    $ tsuru-admin docker-node-add iaas=ec2 image=ami-dc5387b4 region=us-east-1 type=m1.small securityGroup=my-sec-group keyName=my-key
    Node successfully registered.
    $ tsuru-admin docker-node-list
    +-------------------------------------------------------+------------+---------+----------------------------+
    | Address                                               | IaaS ID    | Status  | Metadata                   |
    +-------------------------------------------------------+------------+---------+----------------------------+
    | http://ec2-xxxxxxxxxxxxx.compute-1.amazonaws.com:2375 | i-xxxxxxxx | waiting | iaas=ec2                   |
    |                                                       |            |         | image=ami-dc5387b4         |
    |                                                       |            |         | keyName=my-key             |
    |                                                       |            |         | region=us-east-1           |
    |                                                       |            |         | securityGroup=my-sec-group |
    |                                                       |            |         | type=m1.small              |
    +-------------------------------------------------------+------------+---------+----------------------------+

Manually created nodes
======================

To add a previously provisioned node you call the
`tsuru-admin docker-node-add <http://tsuru-admin.readthedocs.org/en/latest/#docker-node-add>`_ with the ``--register`` flag and setting
the address key with the URL of the Docker API in the remote node.

The docker API must be responding in the referenced address. To instructions
about how to install docker on your node, please refer to `Docker documentation
<https://docs.docker.com/>`_


.. highlight:: bash

::

    $ tsuru-admin docker-node-add --register address=http://node.address.com:2375


