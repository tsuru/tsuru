Node Auto Scaling
=================

Node auto scaling can be enabled by setting `docker:auto-scale:enabled` to true.
It will try to add, remove and rebalance docker nodes used by tsuru.

Node scaling algorithms run in clusters of docker nodes, to specify how clusters
will be formed you must tell tsuru how they should be grouped. This is done by
setting `docker:auto-scale:group-by-metadata` configuration entry to the name of a
metadata present in your nodes.

There are two different scaling algorithms that will be used, depending on how
tsuru is configured: count based scaling, and memory based scaling.

Count based scaling
-------------------

It's chosen if `docker:auto-scale:max-container-count` is set to a value > 0 in
your tsuru configuration.

Adding nodes
++++++++++++

Having `max-container-count` value as :math:`max`, the number of nodes in cluster
as :math:`nodes`, and the total number of containers in all cluster's nodes as
:math:`total`, we get the number of free slots :math:`free` with:

.. math::

    free = max * nodes - total
    
If :math:`free < 0` then a new node will be added and tsuru will rebalance
containers using the new node.

Removing nodes
++++++++++++++

Having `docker:auto-scale:scale-down-ratio` value :math:`ratio`. tsuru will try to
remove an existing node if:

.. math::

    free > max * ratio

Before removing a node tsuru will move it's containers to other nodes available in
the cluster.

To avoid entering loops, removing and adding node, tsuru will require :math:`ratio
> 1`, if this is not true scaling will not run.

Memory based scaling
--------------------

It's chosen if `docker:auto-scale:max-container-count` is not set and your
scheduler is configured to use node's memory information, by setting
`docker:scheduler:total-memory-metadata` and `docker:scheduler:max-used-memory`.

Adding nodes
++++++++++++

Having the amount of memory necessary by the plan with the largest memory
requirement as :math:`maxPlanMemory`. A new node will be added if for all nodes
the amount of unreserved memory (:math:`unreserved`) satisfies:

.. math::

    unreserved < maxPlanMemory


Removing nodes
++++++++++++++

Considering the amount of memory necessary by the plan with the largest memory
requirement as :math:`maxPlanMemory` and `docker:auto-scale:scale-down-ratio`
value as :math:`ratio`. A node will be removed if its current containers can be
distributed across other nodes in the same pool and at least one node still has
unreserved memory (:math:`unreserved`) satisfying:

.. math::

    unreserved > maxPlanMemory * ratio


Rebalancing nodes
-----------------

Rebalancing containers will be triggered when a new node is added or if
rebalancing would decrease the difference of containers in nodes by a number
greater than 2, regardless the scaling algorithm.

Also, rebalancing will not run if `docker:auto-scale:prevent-rebalance` is set to
true.

Auto scale events
-----------------

Each time tsuru tries to run an auto scale action (add, remove, or rebalance). It
will create an auto scale event. This event will record the result of the auto
scale action and possible errors that occurred during its execution.

You can list auto scale events with `tsuru-admin docker-autoscale-list`

Running auto scale once
-----------------------

Even if you have `docker:auto-scale:enabled` set to false, you can make tsuru
trigger the execution of the auto scale algorithm by running `tsuru-admin docker-
autoscale-run`.
