FROM golang:1.25-alpine AS builder

ENV GO111MODULE=on \
    CGO_ENABLED=0

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build --ldflags "-s -w -extldflags -static" -o main .

FROM alpine:3.22

RUN addgroup -S goravel -g 10001 \
    && adduser -S goravel -u 10001 -G goravel \
    && apk add --no-cache ca-certificates wget

WORKDIR /www

COPY --from=builder --chown=goravel:goravel /build/main /www/
COPY --from=builder --chown=goravel:goravel /build/public/ /www/public/
COPY --from=builder --chown=goravel:goravel /build/resources/ /www/resources/

RUN mkdir -p /www/storage/app/public/uploads \
    /www/storage/framework/cache \
    /www/storage/framework/sessions \
    /www/storage/framework/views \
    /www/storage/logs \
    /www/storage/scripts \
    && chown -R goravel:goravel /www/storage

USER goravel

ENTRYPOINT ["/www/main"]
