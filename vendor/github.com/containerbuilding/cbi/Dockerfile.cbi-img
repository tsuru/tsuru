FROM golang:1.10-alpine AS compile
COPY . /go/src/github.com/containerbuilding/cbi
RUN go build -ldflags="-s -w" -o /cbi-img github.com/containerbuilding/cbi/cmd/cbi-img

FROM alpine:3.7
COPY --from=compile /cbi-img /cbi-img
ENTRYPOINT ["/cbi-img"]
