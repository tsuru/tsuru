FROM golang:1.10-alpine AS compile
COPY . /go/src/github.com/containerbuilding/cbi
RUN go build -ldflags="-s -w" -o /cbi-kaniko github.com/containerbuilding/cbi/cmd/cbi-kaniko

FROM alpine:3.7
COPY --from=compile /cbi-kaniko /cbi-kaniko
ENTRYPOINT ["/cbi-kaniko"]
