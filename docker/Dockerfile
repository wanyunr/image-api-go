# 构建阶段：编译纯静态二进制
FROM golang:1.21-alpine AS builder
WORKDIR /build
# 先拷贝依赖文件，利用 Docker 层缓存
COPY src/go.mod src/go.sum ./
RUN go mod download
# 拷贝源码并编译
COPY src/ .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o image-api .

# 运行阶段：最小镜像
FROM alpine:latest
# 安装时区数据，支持 TZ 环境变量
RUN apk add --no-cache tzdata
WORKDIR /app
COPY --from=builder /build/image-api .
EXPOSE 28085
ENTRYPOINT ["./image-api"]
