# Deploy using Dockerfile

Tsuru official's platforms don't support every language, framework, or runtime your tech stack may require.
It doesn't intend to do so.
If you have a setup where the official platforms don't meet your needs, you can deploy your app using its [Dockerfile][Dockerfile reference] (Containerfile).

!!! note "Comparison: container image x Dockerfile"

    Developer's convenience is the key difference between deploying an app using a container image and Dockerfile.
    The former requires extra steps on the developer's side, such as building and publishing the container image, the latter doesn't.

This guide will walk you through the steps to deploy into an app using a simple Dockerfile.

## Prerequisites

This guide assumes that you have:

1. Tsuru client installed (version must be >= 1.15);
2. Set a Tsuru target (server version must be >= 1.14);
3. Logged into Tsuru;
4. An app (henceforth referred as `hello-world-app`) where your user has permission to deploy on.

## Usage

In this example, we are building and deploying a simple application written in Go.
You can checkout the complete code used here on [samples/dockerfile_v1](https://github.com/tsuru/tsuru/main/tree/docs/samples/dockerfile_v1).
You might take a closer look at `Dockerfile` and `main.go` files, but there's no mytery on them.

To deploy into application using its Dockerfile, you just need issue the below command:

``` bash
tsuru app deploy -a hello-world --dockerfile ./
```

The main difference among other types of deploys on Tsuru is the new command argument `--dockerfile`.
The `--dockerfile` command argument allows you to pass a directory or a specific container file.

1. When you pass a directory, Tsuru client tries to find the container file following these names (order matters):

    * `Dockerfile.<app name>` (e.g. `Dockerfile.hello-world-app`)
    * `Containerfile.<app name>` (e.g. `Containerfile.hello-world-app`)
    * `Dockerfile.tsuru`
    * `Containerfile.tsuru`
    * `Dockerfile`
    * `Containerfile`

    Example:
    ```bash
    tsuru app deploy -a <app name> --dockerfile ./path/to/dir/
    ```

2. When you pass a regular file, Tsuru consider it as the container file - regardless of its file name.

    Example:
    ```txt
    tsuru app deploy -a <app name> --dockerfile ./path/to/Dockerfile.custom
    ```

In both cases, the build context (files passed to the builder where you can refer on [COPY](https://docs.docker.com/engine/reference/builder/#copy)/[ADD](https://docs.docker.com/engine/reference/builder/#add) directives) is the current working directory.
Otherwise, you can select only the files you want by passing their namely.

Example:
```
tsuru app deploy -a <app name> --dockerfile ./path/to/Dockerfile file1.txt file2.txt
```

## Advanced tweaks

### How can I access Tsuru env vars during container image build?

You're able to import the env vars configured on Tsuru app while running the deploy with Dockerfile.
To do so, you just need append the following snippet in the Dockerfile.

```dockerfile
RUN --mount=type=secret,id=tsuru-app-envvars,target=/var/run/secrets/envs.sh \
    && . /var/run/secrets/envs.sh \
    ... following commands in this multi-command line are able to see the env vars from Tsuru
```

**NOTE**: That's a bit different than defining `ENV` directive, specially because they're not stored in the image layers.

## Limitations

1. You cannot use distroless based images on your final container image - although on intermediary stages is fine.[^1]
2. There's no support for setting build arguments.
3. There's no support to specify a particular platform - the only platform supported is `linux/amd64`.

[^1]: Tsuru requires a shell intepreter (e.g. `sh` or `bash`) to run hooks, app shell, etc.

[Dockerfile reference]: https://docs.docker.com/engine/reference/builder/
