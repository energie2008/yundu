# syntax=docker/dockerfile:1.7
ARG SERVICE
ARG BUILD_TYPE=service
FROM golang:1.25-alpine AS builder
ARG SERVICE
ARG BUILD_TYPE
RUN apk add --no-cache git gcc musl-dev
WORKDIR /src
COPY go.work go.work.sum ./
COPY apps/ ./apps/
COPY packages/ ./packages/
COPY cmd/ ./cmd/
RUN go work sync
RUN if [ "$BUILD_TYPE" = "migrate" ]; then \
        CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/migrate ./cmd/migrate; \
    else \
        cd /src/apps/${SERVICE} && \
        CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/service ./cmd/... && \
        cd /src && \
        CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/migrate ./cmd/migrate; \
    fi

FROM alpine:3.20
ARG BUILD_TYPE=service
RUN apk add --no-cache ca-certificates tzdata wget bash && \
    cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime && \
    echo "Asia/Shanghai" > /etc/timezone
WORKDIR /app
COPY --from=builder /out/ /app/
COPY migrations /app/migrations
COPY deploy/docker/entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh
EXPOSE 8080-8084 9000
ENV MIGRATE_ON_START=false
ENTRYPOINT ["/app/entrypoint.sh"]
