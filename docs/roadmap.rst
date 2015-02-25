.. Copyright 2015 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

:title: tsuru roadmap.
:description: vision of the future of tsuru.

Tsuru Roadmap
-------------

tsuru release process
=====================
We use github's milestones to releases' planning and **anyone is free** to assign an issue to a milestone,
and discuss about any issue about next tsuru version. We also have internal goals as listed bellow and our focus will be these goals. **But it's not immutable**, we can change any goal at any time as community need.

At globo.com we have goals by quarter of a year (short term goals bellow),
but it **not means we'll have just one release by quarter**.
Our releases have one or more main issues and minor issues which can be minor bugs,
ground work issue and other "not so important but needed" issues.

Everyone is free to assign any issue to any milestones at any time,
and we'll discuss it in the issue or in another communication channel
(gitter.im/tsuru/tsuru or #tsuru channel at irc.freenode.net)

Short term Goals (until July)
=============================
These goals are defined by quarter at globo.com but it can change as comunity need.

    - auto-scale machine pool.

      We need to auto-scale machine pool to be more resilient.

    - `unit-remove` use segreggate schedule.

      Now, the unit-remove command is random and it can create an unbalanced pool,
      so we have to use segregate schedule to remove units more safely.

    - improve `app-swap`

    - improve plugins

    - dockerize tsuru installation.

      We'll run all tsuru components in containers,
      so our installation will be more clean and fast.


Long term Goals
===============
These goals are our goals to 1.0 version.
