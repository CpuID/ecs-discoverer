FROM alpine:3.4

#RUN apk add --no-cache --repository http://dl-3.alpinelinux.org/alpine/edge/community/ go=1.7.1-r0

RUN apk add --no-cache binutils curl git

RUN cd /tmp && \
    curl -o go1.7.1.linux-amd64.tar.gz "https://storage.googleapis.com/golang/go1.7.1.linux-amd64.tar.gz" && \
    tar xzf go1.7.1.linux-amd64.tar.gz && \
    rm -f go1.7.1.linux-amd64.tar.gz

RUN mkdir -p /tmp/gopath && \
    mkdir -p /tmp/ecs-discoverer

ENV PATH "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/tmp/go/bin"

ENV GOPATH "/tmp/gopath"

COPY . /tmp/ecs-discoverer
