## Build
FROM golang as builder

WORKDIR /app

COPY go.mod ./
COPY go.sum ./

RUN go mod download

#COPY internal/ ./internal/
COPY *.go ./

RUN go build -o /action

## Deploy
FROM gcr.io/distroless/base-debian10

WORKDIR /

COPY --from=builder /action /action

ENTRYPOINT ["/action"]