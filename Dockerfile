FROM golang:1.24-alpine AS builder

RUN echo "https://dl-cdn.alpinelinux.org/alpine/v3.15/main" >> /etc/apk/repositories && \
    echo "https://dl-cdn.alpinelinux.org/alpine/v3.15/community" >> /etc/apk/repositories

RUN apk add --no-cache \
    build-base \
    vips-dev \
    pkgconfig

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 go build -ldflags="-w -s" -o /app/scheduler cmd/app/main.go

FROM alpine:3.15

RUN apk add --no-cache vips tzdata
ENV TZ=Asia/Jakarta
RUN addgroup -S app && adduser -S -G app app
WORKDIR /app
COPY --from=builder --chown=app:app /app/scheduler .
USER app
CMD ["./scheduler"]