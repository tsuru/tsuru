// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package juju

var collectOutput = `machines:
  0:
    agent-state: running
    dns-name: 10.10.10.96
    instance-id: i-00000376
    instance-state: running
  97:
    agent-state: running
    dns-name: 10.10.10.189
    instance-id: i-0000040b
    instance-state: running
  100:
    agent-state: running
    dns-name: 10.10.10.208
    instance-id: i-00000422
    instance-state: running
  102:
    agent-state: running
    dns-name: 10.10.10.131
    instance-id: i-00000424
    instance-state: running
  105:
    agent-state: running
    dns-name: 10.10.10.163
    instance-id: i-00000439
    instance-state: running
  107:
    agent-state: running
    dns-name: 10.10.10.168
    instance-id: i-0000043e
    instance-state: running
services:
  as_i_rise:
    charm: local:centos/django-13
    exposed: false
    relations: {}
    units:
      as_i_rise/0:
        agent-state: started
        machine: 105
        public-address: server-1081.novalocal
  the_infanta:
    charm: local:centos/gunicorn-14
    exposed: false
    relations: {}
    units:
      the_infanta/0:
        agent-state: pending
        machine: 107
        public-address: server-1086.novalocal`

var dirtyCollectOutput = `2012-12-14 17:19:28,235 INFO Connecting to environment...
2012-12-14 17:19:29,455 INFO Connected to environment.
machines:
  0:
    agent-state: running
    dns-name: 10.10.10.96
    instance-id: i-00000376
    instance-state: running
  97:
    agent-state: running
    dns-name: 10.10.10.189
    instance-id: i-0000040b
    instance-state: running
  100:
    agent-state: running
    dns-name: 10.10.10.208
    instance-id: i-00000422
    instance-state: running
  102:
    agent-state: running
    dns-name: 10.10.10.131
    instance-id: i-00000424
    instance-state: running
  105:
    agent-state: running
    dns-name: 10.10.10.163
    instance-id: i-00000439
    instance-state: running
  107:
    agent-state: running
    dns-name: 10.10.10.168
    instance-id: i-0000043e
    instance-state: running
services:
  as_i_rise:
    charm: local:centos/django-13
    exposed: false
    relations: {}
    units:
      as_i_rise/0:
        agent-state: started
        machine: 105
        public-address: server-1081.novalocal
  the_infanta:
    charm: local:centos/gunicorn-14
    exposed: false
    relations: {}
    units:
      the_infanta/1:
        agent-state: pending
        machine: 107
        public-address: server-1086.novalocal
2012-12-14 17:19:29,665 INFO 'status' command finished successfully`

var addUnitsOutput = `2012-12-19 14:05:21,275 INFO Connecting to environment...
2012-12-19 14:05:22,681 INFO Connected to environment.
2012-12-19 11:57:31,361 INFO Unit 'resist/3' added to service 'resist'
2012-12-19 11:57:31,550 INFO Unit 'resist/4' added to service 'resist'
2012-12-19 11:57:31,785 INFO Unit 'resist/5' added to service 'resist'
2012-12-19 11:57:31,785 INFO Unit 'resist/6' added to service 'resist'
2012-12-19 14:05:23,251 INFO 'add_unit' command finished successfully`
