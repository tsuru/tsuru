.. Copyright 2015 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

:title: tsuru roadmap.
:description: vision of the future of tsuru.

How tsuru release process works
===============================
We use github's milestones to releases' planning and anyone is free to assign an issue to a milestone,
and discuss about any issue about next tsuru version. We also have internal goals as listed bellow and our focus
will be these goals. But it's not immutable, we can change any goal any time as community need.


Short term Goals (until July)
=============================
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
We dont know, yet! :)
