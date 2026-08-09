# Docker 部署

## 1. 适用场景

适用于希望通过容器快速部署并随时升级服务的场景。当前 Docker 配置会将项目根目录中的配置文件和 URL 列表挂载到容器中，便于直接修改运行配置。

## 2. 前置条件

- Docker 20.10+
- Docker Compose v2
- 项目根目录包含 src/、config.yaml、urls/、docker/

## 3. 部署步骤

```bash
cd docker
cp .env.example .env
cp ../config.yaml.example ../config.yaml

docker compose up -d --build
```

### 验证

```bash
docker compose ps
curl http://127.0.0.1:28085/health
```

## 4. 目录说明

| 文件 | 说明 |
|---|---|
| docker/Dockerfile | 多阶段构建镜像 |
| docker/docker-compose.yml | 容器编排与挂载配置 |
| docker/.env.example | 环境变量模板 |
| .dockerignore | 构建上下文忽略文件 |

## 5. 常用命令

```bash
# 查看日志
docker compose logs -f

# 停止/启动/重启
docker compose stop
docker compose start
docker compose restart

# 重新构建并启动
docker compose up -d --build

# 删除容器但保留镜像
docker compose down

# 进入容器排查
docker compose exec image-api sh
```

## 6. 配置说明

- 修改 docker/.env 可调整端口、时区和代理设置
- 修改项目根目录的 config.yaml / urls/*.txt 后，执行 `docker compose restart` 即可生效
- metadata_mode 默认可配置为 `on_request` 或 `prefetch`

## 7. 常见问题

### 构建超时

若构建时出现 Go 模块下载超时，可在 docker/.env 中配置代理：

```bash
GOPROXY=https://goproxy.cn,direct
```

然后重新执行：

```bash
docker compose up -d --build
```

### 服务返回 503

通常是 URL 列表为空或未成功挂载。请检查：

- urls/portrait.txt、urls/landscape.txt 是否存在
- docker-compose.yml 的挂载配置是否正确
- 文件中是否包含有效 http/https URL

## 8. 版本注入

如需在镜像中注入版本号，可在 Dockerfile 中追加 `-X main.version=...`：

```dockerfile
RUN CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=v1.0.0" -o image-api .
```
