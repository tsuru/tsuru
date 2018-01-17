FROM alpine:3.4
RUN apk update && apk upgrade \
    && apk --no-cache add bash curl git openssh rsyslog \
    && ssh-keygen -A \
    && echo -e "Port 22\nPermitUserEnvironment yes" >> /etc/ssh/sshd_config
RUN adduser -D git \
    && echo git:12345 | chpasswd \
    && mkdir /home/git/.ssh
ADD ./build/ ./bin/
ADD /etc/dockerfile.conf /etc/gandalf.conf
ENV GANDALF_HOST "localhost"
COPY docker-entrypoint.sh /entrypoint.sh
EXPOSE 8000
EXPOSE 22
ENTRYPOINT ["/entrypoint.sh"]