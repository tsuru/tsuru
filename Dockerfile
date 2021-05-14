FROM --platform=$BUILDPLATFORM golang:alpine as builder

ARG TARGETARCH
ENV GOARCH=$TARGETARCH

RUN apk add --no-cache git
COPY . /go/src/github.com/tsuru/tsuru
WORKDIR /go/src/github.com/tsuru/tsuru
ENV GO111MODULE=on
RUN CGO_ENABLED=0 go build -ldflags "-X github.com/tsuru/tsuru/cmd.GitHash=`git rev-parse HEAD`" ./cmd/tsurud/

FROM alpine

RUN apk add --no-cache ca-certificates
COPY --from=builder /go/src/github.com/tsuru/tsuru/tsurud /bin/tsurud
ADD /etc/tsuru-custom.conf /etc/tsuru/tsuru.conf
EXPOSE 8080
ENTRYPOINT ["/bin/tsurud", "api"]
