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

The following configuration register a volume plan called ``ebs`` that is supported kubernetes their own parameters.

.. highlight:: yaml

::

  volume-plans:
    ebs:
      kubernetes:
          storage-class: my-ebs-storage-class

On Kubernetes, volume plans may use a volume plugin or a storage class.

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

Volumes with Minikube
=====================

If you're running `minikube <https://github.com/kubernetes/minikube>`_, you can share a `hostPath <https://kubernetes.io/docs/concepts/storage/volumes/#hostpath>`_ volume among your app units. Add the following configuration to tsuru config file:

.. highlight:: yaml

::

    volume-plans:
      minikube-plan:
        kubernetes:
          storage-class: standard

Then, to create a volume and bind it to your app:

.. highlight:: bash

::

    tsuru volume create my-vol minikube-plan -p my-kubernetes-pool -t my-team -o capacity=1Gi -o access-modes=ReadWriteMany
    tsuru volume bind my-vol /mnt/mountpoint -a my-app
