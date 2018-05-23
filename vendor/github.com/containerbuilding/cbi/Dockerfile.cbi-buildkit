FROM golang:1.10-alpine AS compile
COPY . /go/src/github.com/containerbuilding/cbi
RUN go build -ldflags="-s -w" -o /cbi-buildkit github.com/containerbuilding/cbi/cmd/cbi-buildkit

FROM alpine:3.7
COPY --from=compile /cbi-buildkit /cbi-buildkit
ENTRYPOINT ["/cbi-buildkit"]
