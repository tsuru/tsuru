.. Copyright 2017 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

++++++++
Volumes
++++++++

Volumes allow applications running on tsuru to use external storage volumes mounted on their filesystem.
There are three concepts involved in the process: volume plans, volume and volume binds.

Volume Plans
============

Volume plans are managed by tsuru administrators and are configured in tsuru.conf file. Volume plans describe
how each volume that will be associated to this plan will be created by each provisioner.

The following configuration register a volume plan called ``ebs`` that is supported by swarm and kubernetes using
different parameters. Each has a own set of parameters that may be set on the configuration file.

.. highlight: yaml

::

  volume-plans:
    ebs:
      swarm:
          driver: rexray/ebs
      kubernetes:
          storage-class: my-ebs-storage-class

On swarm a driver must be specified along with its parameters. On Kubernetes, volume plans may use a volume plugin or a storage class.

Volumes
=======

Volumes are created by tsuru users using one of the plans previously configurated. These can be created and managed by using
the tsuru client. The behavior is provisioner specific:

On Kubernetes provisioner
-------------------------

Creating a volume with a plan that has no storage-class defined will cause tsuru to manually create one PersistentVolume 
using the plugin specified in the plan with the opt received in the command line. Also, one PersistentVolumeClaim would be created and bound to 
the PersistentVolume.

If the plan specifies a storage-class instead of a plugin only the PersistentVolumeClaim will be created using the specified storage-class.

On Swarm provisioner
--------------------

A new volume would be created (i.e. docker volume create) using the driver informed in the plan and the volume opt would be a merge between 
the plan opt and command line opt.

Volume binds
============

Volumes binds, like service binds, associate a given application to a previously created volume. This is the moment when
the volume will be made available to the application by the provisioner. The bind/unbind actions can be triggered by the tsuru
client.

Example
=======

Supose an ``ebs`` volume plan is registered in tsuru configuration, one can create a volume using tsuru client:

.. highlight:: bash

::

    $ tsuru volume create myvol ebs -o capacity=1Gi

To be able to use this volume from an app, bind to it:

.. highlight:: bash

::

    $ tsuru volume bind myvol /mnt/mountpoint -a my-app
