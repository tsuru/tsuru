FROM golang:1.10-alpine AS compile
COPY . /go/src/github.com/containerbuilding/cbi
RUN go build -ldflags="-s -w" -o /cbi-buildah github.com/containerbuilding/cbi/cmd/cbi-buildah

FROM alpine:3.7
COPY --from=compile /cbi-buildah /cbi-buildah
ENTRYPOINT ["/cbi-buildah"]
