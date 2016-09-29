FROM alpine:3.4

RUN apk add -U --repository http://dl-3.alpinelinux.org/alpine/edge/community/ go=1.7.1-r0

COPY . /tmp

RUN mkdir -p /tmp/go

ENV GOPATH /tmp/go
