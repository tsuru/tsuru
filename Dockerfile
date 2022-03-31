ARG alpine_version=3.15
ARG golang_version=1.17

FROM --platform=${BUILDPLATFORM} golang:${golang_version}-alpine${alpine_version} AS builder
COPY . /go/src/github.com/tsuru/tsuru
WORKDIR /go/src/github.com/tsuru/tsuru
RUN set -x && \
    apk add --update --no-cache bash git make && \
    make tsurud

FROM alpine:${alpine_version}
COPY --from=builder /go/src/github.com/tsuru/tsuru/tsurud /usr/local/bin/tsurud
COPY /etc/tsuru-custom.conf /etc/tsuru/tsuru.conf
RUN set -x && \
    apk add --no-cache ca-certificates
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/tsurud", "api"]
