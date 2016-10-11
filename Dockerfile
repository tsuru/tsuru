FROM alpine:3.2
RUN apk add --no-cache ca-certificates
ADD tsurud /bin/tsurud
ADD /etc/tsuru-custom.conf /etc/tsuru/tsuru.conf
EXPOSE 8080
ENTRYPOINT ["/bin/tsurud", "api"]
