## Build
FROM golang:1.21-alpine as builder

WORKDIR /app

COPY go.mod ./
COPY go.sum ./

RUN go mod download

COPY *.go ./
COPY internal/ ./internal/
COPY pkg/ ./pkg/

RUN go build -o /action

## Deploy
FROM golang:1.21-alpine

RUN apk update
RUN apk add git

### Install Node
RUN apk add --update --no-cache nodejs npm

### Install Python
RUN apk add --update --no-cache python3 py3-pip

### Install Java
RUN apk add --update --no-cache openjdk11 gradle

### Install Ruby
RUN apk add ruby ruby-bundler

### Install PHP and Composer
RUN apk --update add wget curl php
RUN curl -sS https://getcomposer.org/installer | php -- --install-dir=/usr/bin --filename=composer

WORKDIR /

COPY --from=builder /action /action

ENTRYPOINT ["/action"]