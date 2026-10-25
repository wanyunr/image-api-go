# Docker 部署

## 1. 适用场景

适用于希望通过容器快速部署并随时升级服务的场景。当前 Docker 配置会将项目根目录中的配置文件和 URL 列表挂载到容器中，便于直接修改运行配置。

若不想本地构建，也可以直接使用已发布到 GHCR 的官方镜像（见第 3 节）。

## 2. 前置条件

- Docker 20.10+
- Docker Compose v2
- 官方镜像方式（3.1 / 3.2）：只需准备 `config.yaml` 与 `urls/` 列表（可从 [config.yaml.example](../config.yaml.example) 复制）
- 本地构建方式（3.3）：需要完整源码（src/、docker/）

## 3. 部署步骤

共三种方式，按需选择：

| 方式 | 适用场景 | 需要源码 |
|---|---|---|
| 3.1 docker run + 官方镜像 | 最快体验、临时运行 | 否 |
| 3.2 compose + 官方镜像（推荐） | 长期运行、便于升级与配置管理 | 否 |
| 3.3 compose + 本地构建 | 修改过源码、或无法访问 GHCR | 是 |

官方镜像发布在 `ghcr.io/wanyunr/image-api-go`，双架构（amd64/arm64），支持 `latest` 与版本号标签。

### 3.1 方式一：docker run + 官方镜像

```bash
docker run -d --name image-api \
  -p 28085:28085 \
  -v "$PWD/config.yaml:/app/config.yaml:ro" \
  -v "$PWD/urls:/app/urls:ro" \
  ghcr.io/wanyunr/image-api-go:latest
```

> 不挂载配置直接运行时，容器内会自动生成默认配置与空 URL 列表，服务可启动但图池为空（接口返回 503）。

### 3.2 方式二：compose + 官方镜像（推荐）

新建一个部署目录，把以下内容保存为其中的 `docker-compose.yml`（无需克隆本仓库）：

```yaml
services:
  image-api:
    image: ghcr.io/wanyunr/image-api-go:latest  # 如需固定版本改为 :v1.3.0 等
    container_name: image-api
    ports:
      - "${APP_PORT:-28085}:28085"   # 宿主机端口（可用 .env 覆盖），容器内固定 28085
    environment:
      TZ: "${TZ:-Asia/Shanghai}"     # 容器时区（镜像已装 tzdata）
    volumes:
      - ./config.yaml:/app/config.yaml:ro   # 服务只读配置；URL 列表就绪后 /app/urls 也可加 :ro
      - ./urls:/app/urls
      - ./cache:/app/cache
      # 启用本机图片源时取消注释，并在 config.yaml 的 local.*_dirs 中使用 /app/images/...
      # - ./images:/app/images:ro
    read_only: true
    tmpfs:
      - /tmp
    restart: unless-stopped
```

在部署目录放置 `config.yaml`（可从 [config.yaml.example](../config.yaml.example) 复制）与 `urls/` 列表，然后启动：

```bash
docker compose up -d
```

升级到新版本：

```bash
docker compose pull
docker compose up -d
```

### 3.3 方式三：compose + 本地构建

使用 `docker-compose.yml`，从源码构建镜像：

```bash
git clone https://github.com/wanyunr/image-api-go.git
cd docker
docker compose up -d --build
```

### 3.4 验证

```bash
docker compose ps
curl http://127.0.0.1:28085/health
```

`docker run` 方式（3.1）没有对应 compose 项目，可省略第一条命令。

## 4. 目录说明

| 文件 | 说明 |
|---|---|
| docker/Dockerfile | 多阶段构建镜像（编译静态二进制 + alpine 运行时） |
| docker/docker-compose.yml | 容器编排与挂载配置（本地构建源码） |
| docker/.env.example | 环境变量模板，复制为 .env 使用 |
| .dockerignore | 构建上下文忽略文件 |

官方镜像版的 compose 内容见 3.2 节，按需保存到自己的部署目录，仓库中不单独提供文件。

## 5. 环境变量

在 compose 文件同目录的 `.env` 中配置（本地构建版位于 `docker/.env`；`GOPROXY` 仅本地构建时生效）：

| 变量 | 默认值 | 说明 |
|---|---|---|
| `APP_PORT` | `28085` | 宿主机映射端口，容器内固定 28085 |
| `TZ` | `Asia/Shanghai` | 容器时区，镜像已内置 tzdata |
| `GOPROXY` | 官方代理 | Go 模块代理，仅构建时生效 |

## 6. 常用命令

在 compose 文件所在目录执行（本地构建版为 `docker/`，官方镜像版为你的部署目录）：

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

## 7. 配置生效

- 服务配置的字段含义见 [README 配置速查表](../README.md#配置)，完整注释模板见 [config.yaml.example](../config.yaml.example)
- 修改 `config.yaml` / `urls/*.txt` 后：`docker compose restart` 重启生效，或调用 `GET/POST /admin/reload` 在线热重载（浏览器可直接访问 `?token=…`，URL 列表与本地目录均支持）
- `source: local` 时，需要把宿主机图片目录挂载进容器，并在 `local.portrait_dirs` / `local.landscape_dirs` 中使用容器内路径。docker-compose.yml 已预留 `../images:/app/images:ro` 注释项，按需取消注释

### 7.1 运行权限（防篡改）

容器内服务以**非 root 用户（UID 10001）**运行，且根文件系统只读（`read_only: true`），写操作只发生在挂载卷内。由此：

- `config.yaml` 挂载为只读（`:ro`）——服务从不写配置，篡改面归零
- `urls/` 默认可写（首次启动需要自动生成列表文件）；列表就绪后建议加 `:ro`，彻底封死"篡改 URL 列表注入恶意跳转"的路径
- `cache/` 是唯一需要写权限的挂载（仅 prefetch 模式使用）。宿主机目录需对 UID 10001 可写：

```bash
sudo chown -R 10001:10001 cache
```
- 本地图源目录一律 `:ro` 挂载（compose 中预留的注释项已带）

## 8. 常见问题

### 8.1 构建超时

若构建时出现 Go 模块下载超时，可在 docker/.env 中配置代理：

```bash
GOPROXY=https://goproxy.cn,direct
```

然后重新执行：

```bash
docker compose up -d --build
```

### 8.2 服务返回 503

表示图池为空。`source: "url"` 时检查：

- urls/portrait.txt、urls/landscape.txt 是否存在
- docker-compose.yml 的挂载配置是否正确
- 文件中是否包含有效 http/https URL

`source: "local"` 时检查：

- 图片目录是否已挂载进容器，且 `local.*_dirs` 使用的是容器内路径
- 目录中是否存在被 `local.extensions` 允许的图片文件

## 9. 版本注入

CI 发布的镜像已自动注入与标签一致的版本号（`image-api --version` 可验证）。仅本地手动构建且需要注入版本号时，修改 Dockerfile 的编译行：

```dockerfile
RUN CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=v1.0.0" -o image-api .
```

构建与发布的完整流程见 [built.md](built.md)。
