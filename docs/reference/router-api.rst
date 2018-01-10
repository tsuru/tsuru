.. Copyright 2017 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

++++++++++++++++++++++++
Router API specification
++++++++++++++++++++++++

tsuru supports registering HTTP API routers. An HTTP API router is a generic router
that implements tsuru Router API specification.

The `OpenAPI <https://www.openapis.org/>`_ specification is available at 
`SwaggerHub <https://app.swaggerhub.com/apis/tsuru/tsuru-router_api/1.0.0>`_ 
and as a yaml file :download:`here <router_api.yaml>`.

This specification can be used to generate server stubs and clients. One example of an API
that implements this specification is the `Kubernetes Router <https://github.com/tsuru/kubernetes-router>`_.
