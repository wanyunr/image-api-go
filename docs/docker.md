## 使用教程

### 一、前置条件

1. 已安装 Docker（20.10+）与 Docker Compose v2（`docker compose` 命令）
2. 项目根目录包含 `src/`、`config.yaml`、`urls/`，以及 `docker/` 目录下的部署文件

### 二、文件说明

容器化部署文件统一放在 `docker/` 目录：

| 文件 | 作用 |
|------|------|
| `docker/Dockerfile` | 多阶段构建：`golang:1.21-alpine` 编译 → `alpine` 运行，产物为纯静态二进制 |
| `docker/docker-compose.yml` | 编排：构建镜像、映射端口、环境变量、挂载配置与 URL 列表 |
| `docker/.env.example` | 环境变量示例（端口、时区、Go 模块代理），复制为 `.env` 后生效 |
| `.dockerignore` | 排除无关文件，减小构建上下文（位于项目根，Docker 要求其在构建上下文根目录） |

### 三、构建并启动

```bash
# 进入 docker 目录
cd docker

# 复制环境变量示例（可选，默认值即可用）
cp .env.example .env

# 复制配置文件（必选）
cp ../config.yaml.example ../config.yaml

# 构建镜像并后台启动
docker compose up -d --build

# 查看容器状态
docker compose ps

# 验证服务
curl http://127.0.0.1:28085/health
# {"status":"ok","uptime":"2s","requests_since_start":0,"pool":{"portrait":3,"landscape":3,"total":6}}
```

> 说明：`docker-compose.yml` 的构建上下文指向项目根（源码在 `src/`），因此每次构建都会读取最新的 `src/` 源码。

### 四、常用命令

```bash
# 查看日志（实时跟踪）
docker compose logs -f

# 停止 / 启动 / 重启
docker compose stop
docker compose start
docker compose restart

# 代码变更后重新构建并启动
docker compose up -d --build

# 停止并删除容器（保留镜像）
docker compose down

# 进入容器排查
docker compose exec image-api sh
```

### 五、自定义配置

- **端口**：修改 `docker/.env` 中的 `APP_PORT`（默认 `28085`），容器内端口固定为 `28085`
- **时区**：修改 `docker/.env` 中的 `TZ`（默认 `Asia/Shanghai`）
- **配置与 URL 列表**：直接编辑项目根目录的 `config.yaml` / `urls/*.txt`，已通过数据卷挂载到容器，修改后 `docker compose restart` 生效
- **metadata 模式**：`metadata_mode` 默认 `on_request`（请求 JSON 实时探测）；设为 `prefetch` 启用异步预取+缓存
- **注入版本号**：在 `docker/Dockerfile` 构建命令中追加 `-X main.version=v1.0.0`，例如：
  ```dockerfile
  RUN CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=v1.0.0" -o image-api .
  ```

### 六、常见问题

**1. 构建时 `go mod download` 超时（国内网络）**

Dockerfile 默认使用官方 Go 模块代理 `proxy.golang.org`（Google 服务器），国内直连经常超时：

```
Get "https://proxy.golang.org/...": dial tcp ...:443: i/o timeout
```

**推荐做法**：在 `docker/.env` 中设置代理后正常构建（`docker-compose.yml` 已支持该变量）：

```bash
# docker/.env 中添加：
GOPROXY=https://goproxy.cn,direct

# 然后正常构建启动
docker compose up -d --build
```

**或命令行方式**：注意 `--build-arg` 只属于 `docker compose build`，需分两步：

```bash
docker compose build --build-arg GOPROXY=https://goproxy.cn,direct
docker compose up -d
```

**2. 容器内访问图片返回 503**

`urls/*.txt` 未挂载或列表为空，确认 `docker-compose.yml` 的数据卷挂载正常、文件包含有效的 http/https URL。
