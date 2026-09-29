ARG alpine_version=3.23
ARG golang_version=1.26.6

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

ARG gke_auth_plugin_version=0.6.3
ARG aws_iam_authenticator_version=0.7.20
ARG TARGETARCH
ARG TSURU_BUILD_VERSION
RUN set -x \
  && apk add --update --no-cache curl ca-certificates \
  && curl -fsSL "https://github.com/traviswt/gke-auth-plugin/releases/download/${gke_auth_plugin_version}/gke-auth-plugin_Linux_$( [[ ${TARGETARCH} == 'amd64' ]] && echo 'x86_64' || echo ${TARGETARCH} ).tar.gz" \
  |  tar -C /usr/local/bin -xzvf- gke-auth-plugin \
  && gke-auth-plugin version \
  && curl -fsSL -o /usr/local/bin/aws-iam-authenticator "https://github.com/kubernetes-sigs/aws-iam-authenticator/releases/download/v${aws_iam_authenticator_version}/aws-iam-authenticator_${aws_iam_authenticator_version}_linux_${TARGETARCH}" \
  && chmod +x /usr/local/bin/aws-iam-authenticator \
  && aws-iam-authenticator version
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/tsurud"]
CMD ["api"]
COPY --from=builder /go/src/github.com/tsuru/tsuru/build/tsurud /usr/local/bin/tsurud
