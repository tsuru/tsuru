FROM golang:1.10-alpine AS compile
COPY . /go/src/github.com/containerbuilding/cbi
RUN go build -ldflags="-s -w" -o /cbid github.com/containerbuilding/cbi/cmd/cbid

FROM alpine:3.7
COPY --from=compile /cbid /cbid
ENTRYPOINT ["/cbid"]
