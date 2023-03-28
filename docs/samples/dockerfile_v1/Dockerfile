FROM golang:1.20-alpine3.17 AS builder
COPY ./ /go/app/
RUN set -x \
    && cd /go/app\
    && go build -o server ./main.go

# Remember that you cannot use distroless images.
FROM alpine:3.17
# You should always set the working dir so Tsuru will try to find the config
# files (Procfile, tsuru.yaml) from there.
WORKDIR /var/app
# Copying generated file by other build stage \o/
COPY --from=builder /go/app/server /var/app/
# Copying app's config file to working dir.
COPY ./tsuru.yaml /var/app
# NOTE: Container runtime doesn't expand the env var ($PORT), so we have to
# call a shell interpreter to do so.
#
# See more: https://github.com/moby/moby/issues/5509
CMD ["sh", "-c", "/var/app/server -port $PORT"]
