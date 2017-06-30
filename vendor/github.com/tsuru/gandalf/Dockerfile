FROM alpine:3.2
ADD /webserver/webserver /bin/webserver
ADD /etc/dockerfile.conf /etc/gandalf.conf
EXPOSE 8000
ENTRYPOINT ["/bin/webserver"]
