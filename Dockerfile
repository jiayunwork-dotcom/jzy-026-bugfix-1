# 构建：无 CGO 依赖，静态编译。
FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY internal ./internal
COPY cmd ./cmd
RUN CGO_ENABLED=0 GOOS=linux go vet ./... && \
    CGO_ENABLED=0 GOOS=linux go test ./... && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /camkin ./cmd/camkin

# 运行：单容器，数据目录为本地卷，非 root 运行。
FROM alpine:3.20
RUN adduser -D -u 10001 appuser && mkdir -p /data && chown appuser /data
COPY --from=build /camkin /usr/local/bin/camkin
USER appuser
EXPOSE 8080
ENV DATA_DIR=/data
VOLUME ["/data"]
ENTRYPOINT ["/usr/local/bin/camkin"]
