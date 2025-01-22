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