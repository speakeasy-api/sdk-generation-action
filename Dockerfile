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
RUN apk add git bash curl

### Install Node
RUN apk add --update --no-cache nodejs npm

### Install Python
RUN apk add --update --no-cache python3 py3-pip

### Install Java
RUN apk add --update --no-cache openjdk11 gradle

### Install Ruby
RUN apk add --update --no-cache build-base ruby ruby-bundler ruby-dev


### Install .NET 6.0
ENV DOTNET_ROOT=/usr/lib/dotnet
RUN apk add --update --no-cache dotnet6-sdk

# Install .NET 5.0
RUN curl -sSL https://dot.net/v1/dotnet-install.sh | bash /dev/stdin -Channel 5.0 -InstallDir ${DOTNET_ROOT}
RUN ${DOTNET_ROOT}/dotnet --list-sdks

### Install PHP and Composer
#### Source: https://github.com/geshan/docker-php-composer-alpine/blob/master/Dockerfile
RUN apk --update --no-cache add wget \
		     curl \
		     git \
		     php \
		     php-curl \
		     php-openssl \
		     php-iconv \
		     php-json \
		     php-mbstring \
		     php-phar \
		     php-dom --repository http://nl.alpinelinux.org/alpine/edge/testing/
RUN curl -sS https://getcomposer.org/installer | php -- --install-dir=/usr/bin --filename=composer
RUN mkdir -p /var/www
WORKDIR /var/www
COPY . /var/www
VOLUME /var/www
### END PHP

WORKDIR /

COPY --from=builder /action /action

ENTRYPOINT ["/action"]