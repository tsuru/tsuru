FROM alpine:3.4
RUN apk add --no-cache ca-certificates
ADD build/tsurud /bin/tsurud
ADD /etc/tsuru-compose.conf /etc/tsuru/tsuru.conf
EXPOSE 8080
ENTRYPOINT ["/bin/tsurud", "api"]
