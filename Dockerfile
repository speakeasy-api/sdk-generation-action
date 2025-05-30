## Build
FROM golang:1.23-alpine3.20 as builder

WORKDIR /app

COPY go.mod ./
COPY go.sum ./

RUN go mod download

COPY *.go ./
COPY internal/ ./internal/
COPY pkg/ ./pkg/

RUN go build -o /action

## Deploy
FROM golang:1.23-alpine3.20

RUN apk update

### Install common tools
RUN apk add --update --no-cache bash curl git

### Install Node / NPM
RUN apk add --update --no-cache nodejs npm

### Install Python
RUN apk add --update --no-cache python3 py3-pip python3-dev pipx

### Install Java
RUN apk add --update --no-cache openjdk11 gradle

### Install Ruby
RUN apk add --update --no-cache build-base ruby ruby-bundler ruby-dev

### Install .NET6.0
ENV DOTNET_ROOT=/usr/lib/dotnet
RUN apk add --update --no-cache dotnet6-sdk

### Install .NET8.0
RUN curl -sSL https://dot.net/v1/dotnet-install.sh | bash /dev/stdin -Channel 8.0 -InstallDir ${DOTNET_ROOT}

# Manually install libssl1.1 and libcrypto1.1 because they are required for .NET 5.0 and alpine 3.20 does not have them
RUN ARCH=$(uname -m) && \
    if [ "$ARCH" = "x86_64" ]; then \
        CRYPTO_URL="https://dl-cdn.alpinelinux.org/alpine/v3.16/main/x86_64/libcrypto1.1-1.1.1w-r1.apk"; \
        SSL_URL="https://dl-cdn.alpinelinux.org/alpine/v3.16/main/x86_64/libssl1.1-1.1.1w-r1.apk"; \
    elif [ "$ARCH" = "aarch64" ] || [ "$ARCH" = "arm64" ]; then \
        CRYPTO_URL="https://dl-cdn.alpinelinux.org/alpine/v3.16/main/aarch64/libcrypto1.1-1.1.1w-r1.apk"; \
        SSL_URL="https://dl-cdn.alpinelinux.org/alpine/v3.16/main/aarch64/libssl1.1-1.1.1w-r1.apk"; \
    else \
        echo "Unsupported architecture: $ARCH" && exit 1; \
    fi && \
    echo "Downloading libcrypto1.1 from $CRYPTO_URL" && \
    wget -q -O /tmp/libcrypto1.1.apk "$CRYPTO_URL" && \
    echo "Downloading libssl1.1 from $SSL_URL" && \
    wget -q -O /tmp/libssl1.1.apk "$SSL_URL" && \
    apk add --allow-untrusted /tmp/libcrypto1.1.apk /tmp/libssl1.1.apk && \
    rm /tmp/libcrypto1.1.apk /tmp/libssl1.1.apk && \
    echo "libcrypto1.1 and libssl1.1 installation completed."


# ### Install .NET5.0
RUN curl -sSL https://dot.net/v1/dotnet-install.sh | bash /dev/stdin -Channel 5.0 -InstallDir ${DOTNET_ROOT}
RUN dotnet --list-sdks

### Install PHP and Composer
#### Source: https://github.com/geshan/docker-php-composer-alpine/blob/master/Dockerfile
RUN apk --update --no-cache add \
	wget \
	curl \
	git \
	php83 \
	php83-ctype \
	php83-dom \
	php83-json \
	php83-mbstring \
	php83-phar \
	php83-tokenizer \
	php83-xml \
	php83-xmlwriter \
	php83-curl \
	php83-openssl \
	php83-iconv \
	php83-session \
	php83-fileinfo \
	--repository http://nl.alpinelinux.org/alpine/edge/testing/


RUN curl -sS https://getcomposer.org/installer | php -- --install-dir=/usr/bin --filename=composer
RUN mkdir -p /var/www
WORKDIR /var/www
COPY . /var/www
VOLUME /var/www
### END PHP

WORKDIR /

COPY --from=builder /action /action

ENTRYPOINT ["/action"]