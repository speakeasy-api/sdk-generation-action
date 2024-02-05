## Build
FROM golang:1.21-alpine as builder

### Install Node
RUN apt-get update && apt-get install -y \
    software-properties-common \
    npm
RUN npm install npm@latest -g && \
    npm install n -g && \
    n latest

### Install Python
RUN apt-get install -y python3

### Build Speakeasy Action

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

WORKDIR /

COPY --from=builder /action /action

ENTRYPOINT ["/action"]