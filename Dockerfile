FROM alpine:3.2
ADD tsurud /bin/tsurud
ADD /etc/tsuru-custom.conf /etc/tsuru/tsuru.conf
EXPOSE 8080
ENTRYPOINT ["/bin/tsurud", "api"]
