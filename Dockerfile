FROM golang:1.18 AS build-env

WORKDIR /socket-server

COPY ./* .

RUN go build
