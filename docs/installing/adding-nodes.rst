.. Copyright 2014 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

++++++++++++
Adding Nodes
++++++++++++

Nodes are physical or virtual machines with a Docker installation.

Nodes can be either unmanaged, which mean that they were created manually,  by
provisioning a machine and installing Docker on it, in which case they have to
be registered in tsuru. Or they can be automatically managed by tsuru, which
will handle machine provisioning and Docker installation using your :ref:`IaaS
configuration <iaas_configuration>`.

The managed option is preferred starting with tsuru-server 0.6.0. There are
advantages like automatically healing and scaling of Nodes. The sections below
describe how to add managed and unmanaged nodes.

.. _installing_managed_nodes:

Managed nodes
=============

First step is configuring your IaaS provider in your tsuru.conf file. Please see
the details in :ref:`IaaS configuration <iaas_configuration>`

Assuming you're using EC2, the configuration will be something like:

.. highlight:: yaml

::

  iaas:
    default: ec2
    node-protocol: http
    node-port: 2375
    ec2:
      key-id: xxxxxxxxxxx
      secret-key: yyyyyyyyyyyyy

After you have everything configured, adding a new Docker node is done by
calling `node-add
<http://tsuru-client.readthedocs.io/en/latest/reference.html#add-a-new-node>`_ in
:doc:`tsuru </reference/tsuru-client>` command. This command will receive
a map of key=value params which are IaaS dependent. A list of possible key
params can be seen calling:

.. highlight:: bash

::

    $ tsuru node-add docker iaas=ec2

    EC2 IaaS required params:
      image=<image id>         Image AMI ID
      type=<instance type>     Your template uuid

    Optional params:
      region=<region>          Chosen region, defaults to us-east-1
      securityGroup=<group>    Chosen security group
      keyName=<key name>       Key name for machine


Every key=value pair will be added as a metadata to the Node and you can send
After registering your node, you can list it calling `tsuru node-list <http://tsuru-client.readthedocs.io/en/latest/reference.html#list-nodes-in-cluster>`_

.. highlight:: bash

::

    $ tsuru node-add docker iaas=ec2 image=ami-dc5387b4 region=us-east-1 type=m1.small securityGroup=my-sec-group keyName=my-key
    Node successfully registered.
    $ tsuru node-list
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

Unmanaged nodes
===============

To add a previously provisioned node you call the `tsuru node-add
<http://tsuru-client.readthedocs.io/en/latest/reference.html#add-a-new-node>`_ with the
``--register`` flag and setting the address key with the URL of the Docker API
in the remote node and specify the pool of the node with ``pool=mypoolname``.

The docker API must be responding in the referenced address. To instructions
about how to install docker on your node, please refer to `Docker documentation
<https://docs.docker.com/>`_.


.. highlight:: bash

::

    $ tsuru node-add docker pool=mypoolname --register address=http://node.address.com:2375


To enable the new unmanaged node run this command:

.. highlight:: bash

::

    $ tsuru node-update http://node.address.com:2375 --enable
