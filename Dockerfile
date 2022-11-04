## Build
FROM golang as builder

WORKDIR /app

COPY go.mod ./
COPY go.sum ./

RUN go mod download

#COPY internal/ ./internal/
COPY *.go ./

RUN go build -o /action

RUN /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
RUN echo '# Set PATH, MANPATH, etc., for Homebrew.' >> /root/.profile
RUN echo 'eval "$(/home/linuxbrew/.linuxbrew/bin/brew shellenv)"' >> /root/.profile
RUN eval "$(/home/linuxbrew/.linuxbrew/bin/brew shellenv)" && brew install speakeasy-api/homebrew-tap/speakeasy
RUN eval "$(/home/linuxbrew/.linuxbrew/bin/brew shellenv)" && brew list speakeasy | grep bin/speakeasy | xargs -I '{}' cp '{}' /speakeasy

## Deploy
FROM gcr.io/distroless/base-debian11

WORKDIR /

COPY --from=builder /action /action
COPY --from=builder /speakeasy /speakeasy

ENTRYPOINT ["/action"]