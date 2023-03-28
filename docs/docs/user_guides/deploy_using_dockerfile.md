# Deploy with Dockerfile

As Tsuru may lack support for some platforms (runtime, framework or base OS) your tech stack might require, you can deploy anything else using your own Dockerfile.

This guide will walk you through the steps to successfully deploy into your application using a simple Dockefile.

## Prerequisites

This guide assumes that you have:

1. Tsuru client (`tsuru`) v1.15+ installed;
2. Set a Tsuru target (server);
3. Logged on Tsuru;
4. An app with permission to deploy on.

## Configuring the Dockerfile

!!! note "Restrictions"

    As part of the deployment method on Kubernetes pools, Tsuru requires some tools in the app's container image.
    So you must install at least all of those tools:

    * a shell interpreter, such as `sh` or `bash`;
    * `curl` (optional)


```docker
FROM alpine:v1.16
COPY ./path/to/files /var/app
WORKDIR /var/app
USER nobody
ENTRYPOINT ["/var/app/entrypoint.sh"]
CMD ["--port", "${PORT}"]
```


