FROM golang:1.8.1-alpine

RUN apk add --no-cache git

RUN mkdir -p /tmp/ecs-discoverer

COPY . /tmp/ecs-discoverer
