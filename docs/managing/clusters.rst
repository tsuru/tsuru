.. Copyright 2017 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

++++++++
Clusters
++++++++

Cluster is a concept introduced in tsuru-server 1.2 and allows registering
existing clusters of external provisioners in tsuru. Currently, external
clusters can be registered to ``kubernetes`` provisioner.

Clusters can either have a default flag or have multiple assigned pool. tsuru
will use this information to decide which cluster will be used when interacting
with a pool.

When a cluster is registered in tsuru it means that it becomes visible to the
provisioner as a source of information on nodes and units for applications. It
also become available as a possible destination for the creation of the
resources necessary to deploy or scale an application. See :doc:`provisioners
</managing/provisioners>` for details on which resources are created for each
provisioner.

To manipulate clusters the client commands ``tsuru cluster-add``, ``tsuru
cluster-list``, ``tsuru cluster-update`` and ``tsuru cluster-remove`` can be
used. You can find more information about them in the `client documentation
<http://tsuru-client.readthedocs.io/en/master/reference.html#cluster-management>`_.
