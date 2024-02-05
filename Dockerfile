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
FROM python:3.8.13-alpine3.16 as python

COPY --from=python /usr/local/bin/python3 /usr/local/bin/python3
COPY --from=python /usr/local/lib/python3.8 /usr/local/lib/python3.8
COPY --from=python /usr/local/lib/libpython3.8.so.1.0 /usr/local/lib/libpython3.8.so.1.0
COPY --from=python /usr/local/lib/libpython3.so /usr/local/lib/libpython3.so

WORKDIR /

COPY --from=builder /action /action

ENTRYPOINT ["/action"]