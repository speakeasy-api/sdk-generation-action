## Build
FROM golang as builder

WORKDIR /app

COPY go.mod ./
COPY go.sum ./

RUN go mod download

COPY *.go ./
COPY internal/ ./internal/
COPY pkg/ ./pkg/

RUN go build -o /action

## Deploy
FROM alpine:latest

WORKDIR /

RUN apk update
RUN apk add git

COPY --from=builder /action /action

ENTRYPOINT ["/action"]