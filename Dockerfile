## Build
FROM golang as builder

WORKDIR /app

COPY go.mod ./
COPY go.sum ./

RUN go mod download

COPY *.go ./
COPY internal/ ./internal/

RUN go build -o /action

## Deploy
FROM gcr.io/distroless/base-debian11

WORKDIR /

COPY --from=builder /action /action

ENTRYPOINT ["/action"]