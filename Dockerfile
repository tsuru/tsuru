FROM golang:1.13-alpine3.10 AS builder

COPY ./ /go/src/github.com/tsuru/tsuru

ENV GOPROXY=https://proxy.golang.org

RUN set -x \
    && apk --update add ca-certificates gcc make musl-dev \
    && cd /go/src/github.com/tsuru/tsuru \
    && make tsurud

FROM alpine:3.10

COPY --from=builder /go/src/github.com/tsuru/tsuru/build/tsurud /usr/bin/tsurud

ARG tsuru_user=tsuru
ARG tsuru_uid=10000
ARG tsuru_gid=10000

RUN set -x \
    && apk add --update --no-cache ca-certificates \
    && addgroup -S -g ${tsuru_gid} ${tsuru_user} \
    && adduser -DS -h /etc/tsuru -s /usr/bin/nologin -u ${tsuru_uid} -G ${tsuru_user} ${tsuru_user} \
    && chown -R ${tsuru_user}:${tsuru_user} /etc/tsuru

USER ${tsuru_user}

WORKDIR /etc/tsuru

EXPOSE 8080

CMD ["tsurud", "api"]
