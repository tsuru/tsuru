# Deploying with Dockerfile

This example shows a basic application written in Go that runs a web server on port `$PORT` (following Tsuru best practices).
It responds for two endpoints `/` (prefix) and `/healthz`, such as greets the caller and returns a "working" sentence respectively.

Details:
* Its Dockerfile uses the multi-stage build strategy which generates a lightweight container image;
* The final container image is small but it cannot be distroless (scratch-like);
* The app config file `tsuru.yaml` is copied to the container image's working dir which meant Tsuru may set more configurations for the app, such as health check.

You can deploy this example into an app using the command:

```bash
$ tsuru app deploy -a <APP> --dockerfile .
```
