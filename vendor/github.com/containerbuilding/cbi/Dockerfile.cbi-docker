FROM golang:1.10-alpine AS compile
COPY . /go/src/github.com/containerbuilding/cbi
RUN go build -ldflags="-s -w" -o /cbi-docker github.com/containerbuilding/cbi/cmd/cbi-docker

FROM alpine:3.7
COPY --from=compile /cbi-docker /cbi-docker
ENTRYPOINT ["/cbi-docker"]
