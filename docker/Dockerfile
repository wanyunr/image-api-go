# 构建阶段：编译纯静态二进制
FROM golang:1.25-alpine AS builder
WORKDIR /build
# 默认使用官方 Go 模块代理；国内网络构建超时时，用 --build-arg 覆盖为国内镜像：
#   docker compose up -d --build --build-arg GOPROXY=https://goproxy.cn,direct
ARG GOPROXY=https://proxy.golang.org,direct
ENV GOPROXY=${GOPROXY}
# 拷贝依赖文件
COPY src/go.mod src/go.sum ./
RUN go mod download
# 拷贝源码并编译
COPY src/ .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o image-api .

# 运行阶段：最小镜像
FROM alpine:latest
# 安装时区数据，支持 TZ 环境变量
RUN apk add --no-cache tzdata \
    && adduser -D -H -u 10001 app
WORKDIR /app
COPY --from=builder /build/image-api .
RUN mkdir -p /app/cache && chown -R app:app /app
USER app
EXPOSE 28085
ENTRYPOINT ["./image-api"]
