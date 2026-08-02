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
| `docker/.env.example` | 环境变量示例（端口、时区），复制为 `.env` 后生效 |
| `.dockerignore` | 排除无关文件，减小构建上下文（位于项目根，Docker 要求其在构建上下文根目录） |

### 三、构建并启动

```bash
# 进入 docker 目录
cd docker

# 复制环境变量示例（可选，默认值即可用）
cp .env.example .env

# 构建镜像并后台启动
docker compose up -d --build

# 查看容器状态
docker compose ps

# 验证服务
curl http://127.0.0.1:28085/health
# {"pool":{...},"status":"ok",...}
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
- **热重载 URL 池**（无需重启）：`curl -X POST http://127.0.0.1:28085/admin/reload`
- **注入版本号**：在 `docker/Dockerfile` 构建命令中追加 `-X main.version=v1.0.0`，例如：
  ```dockerfile
  RUN CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=v1.0.0" -o image-api .
  ```
