# App deploy

The most common way of deploying a tsuru application is trough tsuru's command line: `tsuru app deploy`.

## Prerequisites

1. Tsuru client installed (version must be >= 1.15);
2. Set a Tsuru target (server version must be >= 1.14);
3. Logged into Tsuru;

## Usage

The first step to deploy using the `app deploy` command is to create an app, this step is quite important, because when creating an app you need to specify the app's platform.

The platform will define how to effectivelly deploy the app. If your application can be compiled into a binary file, it may be of interest to you to just deploy the compiled binary with minimal overhead.
But if you're developing a Django application, you may need to deploy an entire directory for it to work properly.

The steps to deploy both applications are almost analogous, key differences will be highlighted when necessary.

1. Let's start with the django application example, you can create your app using tsuru's maintained [python](https://github.com/tsuru/platforms/tree/master/python) platform:

    ```bash
    tsuru app create django-app python
    ```

    !!! note "Tsuru supported and mantained platforms"

        Tsuru keeps a [github repository](https://github.com/tsuru/platforms) with all it's suported platforms, along with with guides on how to deploy on each of them.

2. Lastly, just call `app deploy` passing the entire app directory as an argument:

    ```bash
    tsuru app deploy -a django-app .
    ```

!!! note

    In the case of the aforementioned compiled binary example, you'd create the app using the scratch platform, compile it, and pass the binary file location to the command line:

    ```bash
    GOOS=linux GOARCH=amd64 go build -o myapp main.go
    ```

    ```bash
    tsuru app deploy -a binary-app ./myapp
    ```

## Customization

### - Procfile

Tsuru allows you to specify *how* your app should run, this is mainly achieved by writing a [Procfile](link to procfile reference) and sending it along with your deployed files. A basic Procfile can be seen bellow:

```yaml
web: gunicorn -b 0.0.0.0:$PORT blog.wsgi
```

In this example Procfile we tell tsuru how the process we called "web" should be run: we will use gunicorn to run it, using the local address `0.0.0.0` on the port specified by the `$PORT` environment variable

### - tsuru.yaml

tsuru.yaml is a special file located in the root of the application. The name of the file may be `tsuru.yaml` or `tsuru.yml`.

This file is used to describe certain aspects of your app. Currently it describes information about deployment hooks and deployment time health checks.

- Deployment hooks

    tsuru provides some deployment hooks, like `restart:before`, `restart:after` and `build`. Deployment hooks allow developers to run commands before and after some commands.

    Here is an example about how to declare this hooks in your tsuru.yaml file:

    ```yaml
    hooks:
    restart:
        before:
        - python manage.py generate_local_file
        after:
        - python manage.py clear_local_cache
    build:
        - python manage.py collectstatic --noinput
        - python manage.py compress
    ```

- Healthcheck

    You can declare a health check in your tsuru.yaml file. This health check will be called during the deployment process and tsuru will make sure this health check is passing before continuing with the deployment process.

    If tsuru fails to run the health check successfully it will abort the deployment before switching the router to point to the new units, so your application will never be unresponsive.

    A simple example of a healthcheck command to be excuted can be seen bellow:

    ```yaml
    healthcheck:
        command: ["curl", "-f", "-XPOST", "http://localhost:8888"]
    ```

- More customization

    Even more customization can be achieved with tsuru.yaml, i.e: which ports will be exposed on each process of your app.

    Going trough all the options in this `app deploy` guide would make things cumbersome, that's why we keep a [reference](link to references) page that explains every option available in `Procfile` and `tsuru.yaml` in detail.
