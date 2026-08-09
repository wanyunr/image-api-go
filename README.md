# Image API

轻量级随机图片 API，使用 Go 及官方扩展库构建，零框架、无数据库依赖。图片元数据探测支持 WebP（`golang.org/x/image`）。

访问 API 即自动 302 重定向到一张随机图片，浏览器直接展示，图片不经过本服务器。

## 项目结构

```
image-api/
├── src/                  # Go 工程（模块根目录）
│   ├── main.go           # 入口
│   ├── go.mod / go.sum
│   └── app/              # 业务逻辑包
│       ├── config.go     # 配置加载与校验
│       ├── handler.go    # HTTP 处理器
│       ├── pool.go       # URL 池管理
│       ├── recent.go     # 最近访问追溯
│       ├── imageinfo.go  # 图片元数据探测（支持 WebP）
│       ├── cache.go      # metadata 缓存持久化
│       ├── ratelimit.go  # 全局限流
│       └── util.go       # 通用工具
├── config.yaml           # 配置文件
├── docs/                 # 项目文档
├── docker/               # 容器化部署
│   ├── Dockerfile
│   ├── docker-compose.yml
│   └── .env.example
└── LICENSE
```

## 快速开始

```bash
# 编译（在 src 目录内执行，产物输出到项目根目录）
cd src
go build -o ../image-api .

# 运行（回到项目根目录，默认读取 config.yaml）
cd ..
./image-api

# 指定配置文件
./image-api /path/to/config.yaml
```

> 说明：Go 模块根目录为 `src/`，请在 `src/` 内执行 `go build` / `go run` / `go test` 等命令。所有运行时路径（配置文件、URL 列表）以**当前工作目录**为基准，推荐在项目根目录运行二进制。

> 生产环境部署（systemd 服务、开机自启、日志管理）见 [docs/systemd.md](docs/systemd.md)。
> Docker Compose 一键构建启动见 [docs/docker.md](docs/docker.md)。

## 开发调试

**前提：**

1. `go` 命令（build / run / test / vet）必须在 `src/` 目录内执行 —— Go 模块根目录在 `src/`，在根目录执行会报 `cannot find main module`。
2. 程序运行时的配置与 URL 列表路径以**当前工作目录**为基准。直接从 `src/` 执行 `go run .` 会读不到根目录的 `config.yaml` / `urls/`，程序会在此目录自动生成空的默认文件，导致 URL 池为空（访问返回 503）。

**推荐调试方式：在 `src/` 构建，回到根目录运行：**

```bash
# 一键构建并启动（在项目根目录执行）
cd src && go build -o ../image-api . && cd .. && ./image-api

# 另开终端验证
curl http://127.0.0.1:28085/health    # {"pool":{...},"status":"ok",...}
curl -i http://127.0.0.1:28085/       # 302 → 随机图片

# 停止服务：Ctrl + C
```

**单元测试与静态检查**（在 `src/` 内）：

```bash
cd src
go test ./...        # 运行全部测试
go vet ./...         # 静态检查
go test -race ./...  # 竞态检测（需要 CGO_ENABLED=1；Windows 需安装 gcc）
```

**日志排查：** 程序日志全部输出到标准输出（stdout）。启动时依次打印：配置加载、URL 池数量（portrait/landscape）、监听地址、各功能开关（鉴权/限流/防盗链）；每个请求打印一行 `[request] IP=... UA=... Query=...`（`Query` 中的 `token` 参数值已自动脱敏为 `***`），直接在终端观察即可。

**运行参数：**

```bash
./image-api                      # 默认读取 config.yaml
./image-api /path/to/config.yaml # 指定配置文件
./image-api --version            # 查看版本（-v 亦可）
```

**热重载：** 修改 `urls/*.txt` 后无需重启，调用 `curl http://127.0.0.1:28085/admin/reload` 即可刷新 URL 池。

## 配置说明

```yaml
server:
  host: "0.0.0.0"
  port: 28085
  # base_path: "/"              # 反代子目录前缀（可选，默认空=无前缀）
  cache_dir: "cache"            # prefetch 模式下的 metadata 缓存目录
  metadata_mode: "on_request"   # on_request（默认）：实时探测；prefetch：异步预取+缓存
  # recent_buffer_size: 50      # 最近访问追溯条数（默认 50，GET /admin/recent）

urls:
  portrait: "urls/portrait.txt"    # 竖屏 URL 列表
  landscape: "urls/landscape.txt"  # 横屏 URL 列表

auth:
  enabled: false                   # Token 鉴权开关
  tokens:
    - "your-secret-token"

rate_limit:
  enabled: false                   # 频率限制开关
  per_minute: 30
  per_hour: 120
  per_day: 300

referer:
  enabled: false                   # Referer 防盗链开关
  allow_empty: true                # 是否允许空 Referer
  whitelist:
    - "example.com"
    - "*.example.com"              # 支持通配符
```

## URL 文件格式

```text
# 注释行，自动忽略
# 空行自动忽略

https://cdn.example.com/img/001.jpg
https://cdn.example.com/img/002.jpg
```

- 一行一个 URL
- 自动忽略空行和 `#` 开头的注释
- 自动校验 URL 格式（必须为 http/https）

## API 接口

### 获取随机图片

```
GET /
```

> 若配置了 `base_path`（如 `/api`），端点前缀相应变化：`/api/`、`/api/health`、`/api/admin/reload`。

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `orientation` | `portrait`（竖屏）/ `landscape`（横屏） | 根据 UA 自动判断 |
| `format` | `redirect` / `json` / `text` | `redirect` |
| `token` | 鉴权 Token（开启鉴权时必填） | - |

**自动适配逻辑：** 未指定 `orientation` 时，根据 User-Agent 判断设备类型：
- 手机 → portrait
- PC / 平板 → landscape

**返回格式：**

| format | 行为 |
|--------|------|
| 默认 / `redirect` | 302 重定向到图片 URL，浏览器直接展示图片 |
| `json` | 返回 `{"url":"...","orientation":"..."}` |
| `text` | 返回纯文本 URL（适合 curl / 脚本） |

**示例：**

```bash
# 302 跳转
curl -i "http://localhost:28085/"
# HTTP/1.1 302 Found
# Location: https://cdn.example.com/p/001.jpg

# 强制竖屏
curl -i "http://localhost:28085/?orientation=portrait"

# JSON 格式
curl "http://localhost:28085/?format=json"
# {"url":"https://cdn.example.com/p/001.jpg","orientation":"landscape","width":1920,"height":1080,"format":"jpeg","filename":"001.jpg"}

# 纯文本
curl "http://localhost:28085/?format=text"
# https://cdn.example.com/p/001.jpg

# 带 Token
curl "http://localhost:28085/?token=your-secret-token&format=json"
# {"url":"...","orientation":"portrait","width":1080,"height":1920,"format":"webp","filename":"001.webp"}
```

### 健康检查

```
GET /health
```

```bash
curl "http://localhost:28085/health"
# {"status":"ok","uptime":"2s","requests_since_start":0,"pool":{"portrait":3,"landscape":3,"total":6,"pending":0,"dead":0}}
```

### 热重载 URL 列表

```
GET /admin/reload
```

修改 `portrait.txt` / `landscape.txt` 后无需重启，调用此接口即可刷新内存中的 URL 池。

```bash
curl -H "Authorization: Bearer your-admin-token" "http://localhost:28085/admin/reload"
# {"status":"reloaded","portrait":5,"landscape":8,"pending":2,"dead":0}
```

### 最近访问追溯

```
GET /admin/recent
```

查看最近返回的图片记录（按时间倒序），避免喜欢的图找不到。

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `limit` | 返回条数 | 全部 |

```bash
curl -H "Authorization: Bearer your-admin-token" "http://localhost:28085/admin/recent?limit=20"
# {"total":34,"items":[{"url":"...","orientation":"portrait","format":"redirect","at":"..."},...]}
```

缓冲条数由 `server.recent_buffer_size` 配置（默认 50），仅内存保留，重启后清空。

### 清空死链

```
POST /admin/clear-dead
```

prefetch 模式下连续失败 3 次的 URL 不再重试，此接口清空后下次 reload 重新尝试。

```bash
curl -X POST -H "Authorization: Bearer your-admin-token" "http://localhost:28085/admin/clear-dead"
# {"status":"dead links cleared"}
```

> `/admin/*` 始终要求 Token（`auth.admin_token` 优先，否则回退 `auth.tokens`）。

## 鉴权

开启 `auth.enabled: true` 后，所有请求需携带 Token：

```bash
# Header 方式（推荐：Token 不会进入 URL、代理日志与浏览器历史）
curl -H "Authorization: Bearer your-secret-token" http://localhost:28085/

# Query 参数方式（方便调试；注意 Token 会出现在访问日志中，生产环境慎用）
curl "http://localhost:28085/?token=your-secret-token"
```

未授权返回 `401 Unauthorized`。

## 频率限制

开启 `rate_limit.enabled: true` 后，**整个服务所有请求共享统一额度**（不按 IP 分桶），支持三级限制同时生效：

- 每分钟 / 每小时 / 每天

超限返回 `429 Too Many Requests`。

## Referer 防盗链

开启 `referer.enabled: true` 后，仅允许白名单内的 Referer 访问：

- 支持精确匹配：`example.com`
- 支持通配符：`*.example.com`
- `allow_empty: true` 允许无 Referer 的请求（如直接访问、curl）

非法 Referer 返回 `403 Forbidden`。

## 错误码

| HTTP 状态码 | 含义 |
|------------|------|
| 400 | 参数错误（orientation / format 无效） |
| 401 | Token 鉴权失败 |
| 403 | Referer 防盗链拦截 |
| 429 | 超过访问频率限制 |
| 500 | 服务器内部错误 |
| 503 | URL 池为空 |

## 性能设计

- URL 列表启动时一次性加载到内存，请求时不读磁盘
- **`on_request` 模式（默认）**：零启动时间，`format=json` 时实时探测图片尺寸
- **`prefetch` 模式**：缓存命中秒启动，后台异步补抓全量 metadata；适用于大量 URL 需要快速 `format=json` 响应的场景
- 图片不经过本服务器，仅返回 302 重定向
- 仅依赖 `gopkg.in/yaml.v3` 与官方扩展库 `golang.org/x/image`（WebP 解码），无第三方框架

**推荐部署架构：**

```
Client → Cloudflare / Nginx → Go API → 返回 CDN/R2/OSS 图片 URL
```

> 限流为全局限额（与来源 IP 无关），伪造转发头无法绕过；若需要按来源限制，请在前置 Nginx / Cloudflare 层配置。

### 反代子目录部署

如果希望以 `https://example.com/api/` 这样的**子目录路径**反代到本服务，无需在 Nginx 侧做路径 rewrite，只需在 `config.yaml` 中配置 `server.base_path`：

```yaml
server:
  base_path: "/api"    # 路由自动变为 /api/、/api/health、/api/admin/reload
```

**Nginx 反代配置**（前面**不做 rewrite**，原样转发即可）：

```nginx
location /api/ {
    proxy_pass http://127.0.0.1:28085/api/;   # 路径不动，后端自己识别前缀
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
}
```

**请求路径映射**（`base_path: "/api"` 时）：

| 用户访问 | 反代转发 | 后端路由（base_path 生效后） |
|----------|----------|-------------------------------|
| `example.com/api/` | `/api/` | `/api/` → 正常响应 |
| `example.com/api/?format=json` | `/api/?format=json` | `/api/` → 正常响应 |
| `example.com/api/health` | `/api/health` | `/api/health` → 健康检查 |

> 原理：`http.StripPrefix` 在请求到达 handler 前自动剥掉前缀，handler 内部逻辑完全不受影响。默认 `base_path` 为空（等同于 `/`），行为与之前完全一致。