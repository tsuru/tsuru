FROM alpine:3.2
ADD /cmd/tsurud/tsurud /bin/tsurud
ADD /etc/dockerfile.conf /etc/tsuru/tsuru.conf
EXPOSE 8080
ENTRYPOINT ["/bin/tsurud", "api"]
