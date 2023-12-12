ARG alpine_version=3.17
ARG golang_version=1.20

FROM golang:${golang_version}-alpine${alpine_version} AS builder
RUN apk add --update --no-cache bash make git
WORKDIR /go/src/github.com/tsuru/tsuru
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN make tsurud

FROM alpine:${alpine_version}
RUN set -x && \
    apk add --no-cache ca-certificates
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/tsurud"]
CMD ["api"]
COPY --from=builder /go/src/github.com/tsuru/tsuru/build/tsurud /usr/local/bin/tsurud
COPY gke-auth-plugin_Linux_x86_64.tar.gz /tmp
RUN cd /tmp && tar -C /usr/local/bin -xzvf gke-auth-plugin_Linux_x86_64.tar.gz gke-auth-plugin \
    && gke-auth-plugin version