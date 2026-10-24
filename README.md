# Image API

## 概览

Image API 是一个轻量级随机图片服务，基于 Go 标准库和内存 URL 池构建。默认不代理图片流量，只返回一条随机图片地址，适合接入 CDN、OSS 或对象存储；也可以启用本机图片目录，由服务直接返回文件。

### 核心能力

- 默认返回 302 跳转到外部图片 URL
- 支持 portrait / landscape 两种方向
- 支持 JSON、TEXT、REDIRECT 三种输出格式（`source: "local"` 时仅支持 REDIRECT）
- 支持本机图片目录作为图源（直接返回文件）
- 支持 Token 鉴权、全局限流、防盗链
- 支持热重载、健康检查、Prometheus 指标

## 项目结构

```text
image-api/
├── src/                  # Go 模块根目录
│   ├── main.go           # 服务入口
│   ├── go.mod            # Go 模块定义
│   └── app/              # 业务逻辑包
│       ├── config.go     # 配置加载与校验
│       ├── handler.go    # HTTP 处理器
│       ├── pool.go       # URL 池管理
│       ├── recent.go     # 最近访问追溯
│       ├── imageinfo.go  # 图片元数据探测（支持 WebP）
│       ├── cache.go      # metadata 缓存持久化
│       ├── ratelimit.go  # 全局限流
│       └── util.go       # 通用工具
├── config.yaml           # 运行时配置文件
├── urls/                 # URL 列表目录（source: "url"）
├── images/               # 本机图片目录（source: "local"）
├── docs/                 # 文档目录
├── docker/               # Docker 部署文件
└── LICENSE
```

## 快速开始

### 1. 准备配置与 URL 列表

如果根目录下还没有配置文件，请先复制示例文件：

```bash
cp config.yaml.example config.yaml
```

默认 `source: "url"`，在 URL 列表里每行放一个图片地址：

```bash
ls urls/portrait.txt urls/landscape.txt
```

如果要改成直接返回本机图片文件，把 `config.yaml` 中的 `source` 改为 `"local"`，并把图片按方向放进对应目录：

```bash
mkdir -p images/portrait images/landscape
ls images/portrait images/landscape
```

### 2. 构建并运行

```bash
cd src
go build -o ../image-api .
cd ..
./image-api
```

### 3. 验证服务

```bash
curl http://127.0.0.1:28085/health
curl -i http://127.0.0.1:28085/
```

> 说明：Go 模块根目录是 src/。建议在项目根目录运行生成的二进制文件，这样配置和 URL 列表路径会按预期读取。

## 配置说明

下面这份配置是当前版本默认可用的完整示例，字段含义都写在注释里，可以直接复制为 `config.yaml` 使用。

```yaml
# 图片来源：url（URL 列表，默认）| local（本机目录），二选一
source: "url"

server:
  # 服务监听地址，0.0.0.0 表示监听所有网卡
  host: "0.0.0.0"
  # HTTP 监听端口
  port: 28085
  # metadata 缓存目录；prefetch 模式下会把已抓取的信息持久化到这里
  cache_dir: "cache"
  # metadata 获取模式：
  #   on_request  -> 请求时实时探测，启动更快（默认）
  #   prefetch    -> 启动时预取并后台异步补抓，适合较大 URL 池
  metadata_mode: "on_request"
  # 反向代理子路径前缀，例如配置为 /api 后，路由会变为 /api/、/api/health
  base_path: "/"
  # 最近访问记录缓冲区大小，默认 50，重启后清空
  # recent_buffer_size: 50

urls:
  # 竖屏图片 URL 列表文件
  portrait: "urls/portrait.txt"
  # 横屏图片 URL 列表文件
  landscape: "urls/landscape.txt"

local:
  # 参与扫描的图片扩展名，不区分大小写
  extensions: [".jpg", ".jpeg", ".png", ".webp", ".gif"]
  # 竖屏 / 横屏图片目录，仅在 source: "local" 时生效
  portrait_dirs: ["images/portrait"]
  landscape_dirs: ["images/landscape"]

auth:
  # 是否启用 Token 鉴权
  enabled: false
  # 访问接口时允许使用的 Token 列表
  tokens:
    - "your_secret_token"
  # 管理接口专用 Token，优先级高于 auth.tokens
  # admin_token: "your_admin_token"

rate_limit:
  # 是否启用全局限流
  enabled: false
  # 每分钟最大请求数
  per_minute: 30
  # 每小时最大请求数
  per_hour: 120
  # 每天最大请求数
  per_day: 300

referer:
  # 是否启用 Referer 防盗链
  enabled: false
  # 是否允许空 Referer
  allow_empty: true
  # 允许访问的 Referer 白名单，支持精确匹配和通配符
  whitelist:
    - "example.com"
    - "*.example.com"
```

### 配置项说明

- `server.host`：监听地址，通常保持 `0.0.0.0`。
- `server.port`：HTTP 服务端口，默认 `28085`。
- `server.cache_dir`：prefetch 模式下缓存 metadata 的目录。
- `server.metadata_mode`：
  - `on_request`：默认值，启动更快；请求 `format=json` 时实时探测图片尺寸。
  - `prefetch`：启动时把 URL 列表预载入，并后台补抓 metadata。
- `server.base_path`：给 Nginx / Caddy 反向代理子路径使用。
- `urls.portrait` / `urls.landscape`：分别指定竖屏和横屏 URL 列表文件，仅在 `source: "url"` 时生效。
- `source`：图片来源，`url`（默认，读取 URL 列表）或 `local`（扫描本机目录），二选一。未声明时按 `url` 处理，与旧版本行为一致。
- `local.extensions`：参与扫描的图片扩展名，不区分大小写；为空时回退到默认值。
- `local.portrait_dirs` / `local.landscape_dirs`：按方向配置的本机图片目录，启动和 `/admin/reload` 时重新扫描。目录内的隐藏文件与隐藏子目录（如 `.DS_Store`、`.thumbnails/`）会被跳过；路径按服务进程的工作目录解析。
- `auth.enabled`：开启后接口请求必须携带 Token。
- `auth.tokens`：普通接口可使用的 Token。
- `auth.admin_token`：管理接口专用 Token，优先级更高。
- `rate_limit.enabled`：开启后对全局请求进行限流。
- `referer.enabled`：开启后限制 Referer 白名单。
- `referer.allow_empty`：是否允许空 Referer 访问。

## URL 列表格式

`source: "url"`（默认）时使用。每行一个 URL，支持空行和注释：

```text
# 注释行
https://cdn.example.com/img/001.jpg
https://cdn.example.com/img/002.jpg
```

## API 说明

### 1. 获取随机图片

```http
GET /
```

这是核心接口。默认会返回一张随机图片地址，并根据情况自动选择 portrait 或 landscape；`source: "local"` 时直接返回文件内容。

#### 可用参数

| 参数 | 说明 | 默认值 |
|---|---|---|
| orientation | 指定方向：`portrait` 或 `landscape` | 未传时根据 User-Agent 自动判断 |
| format | 输出格式：`redirect` / `json` / `text` | `redirect` |
| token | 鉴权 Token；开启鉴权时必填 | 无 |

#### 自动判断逻辑

- 手机类 UA → `portrait`
- 电脑 / 平板类 UA → `landscape`

#### 返回格式

| format | 行为 |
|---|---|
| `redirect` | 返回 302 跳转，浏览器直接展示图片 |
| `json` | 返回包含 URL、尺寸、格式、文件名的 JSON |
| `text` | 返回图片 URL 的纯文本 |

`source: "local"` 时只支持直接输出文件：

- `redirect`：直接返回本地文件内容（200），不发跳转。
- `json` / `text`：一律返回 400 —— 本地图源没有可交出去的外部地址，请改用 `redirect`。

#### 示例

```bash
# 默认跳转
curl -i http://localhost:28085/

# 强制竖屏
curl -i "http://localhost:28085/?orientation=portrait"

# 返回 JSON
curl "http://localhost:28085/?format=json"

# 返回纯文本 URL
curl "http://localhost:28085/?format=text"

# 开启鉴权后使用 Token
curl -H "Authorization: Bearer your_secret_token" "http://localhost:28085/?format=json"
## 也可通过 query 参数传递 Token
curl "http://localhost:28085/?token=your-secret-token&format=json"
```

### 2. 健康检查

```http
GET /health
```

返回当前服务状态、运行时长、池子数量和 pending/dead 统计。

```bash
curl http://localhost:28085/health
```

### 3. 本地图片静态访问

```http
GET /local/<id>/<filename>
```

仅在 `source: "local"` 时注册。`<id>` 是启动扫描时按文件绝对路径算出的哈希，只有被扫描到的文件才能通过这个路由访问，因此不存在目录穿越风险；目录内的隐藏文件与符号链接不会进入白名单。

- 随机接口直接返回文件内容，不暴露该地址；它主要出现在 `/admin/recent` 的记录里，便于排查"刚才返回的是哪张图"。
- 同样受 `auth`、`referer`、`rate_limit` 配置约束。
- 文件在扫描后被删除时，随机接口会自动重选；全部文件都缺失时返回 503，`/admin/reload` 可重新扫描。文件在扫描后被替换为符号链接时同样按不可用处理（读取阶段会校验路径仍指向同一普通文件）。
- 启动 → 扫描 → 请求处理 → 异常分支的完整流程图见 [docs/development.md](docs/development.md#本地图源source-local流程)。

### 4. 管理接口

管理接口专门用于刷新池子、清理死链和查看最近访问记录。它们优先使用 `auth.admin_token` 进行鉴权；如果没有配置 `admin_token`，则仅当 `auth.enabled: true` 时才会回退到普通 `auth.tokens` 中的 Token。若 `auth.enabled: false` 且未配置 `admin_token`，管理接口将返回 401。

#### 热重载 URL 池

```http
POST /admin/reload
```

重新加载 URL 列表（`source: "local"` 时重新扫描图片目录），并刷新内存池。适用于修改了 `urls/*.txt` 或增删了本地图片后立即生效。

```bash
# 使用 admin_token
curl -X POST -H "Authorization: Bearer your_admin_token" http://localhost:28085/admin/reload

# 如果未配置 admin_token，则使用普通 Token
curl -X POST -H "Authorization: Bearer your_secret_token" http://localhost:28085/admin/reload
```

#### 清空死链

```http
POST /admin/clear-dead
```

清空 prefetch 模式下已经判定为死链的 URL，下一次重载时会重新尝试。

```bash
curl -X POST -H "Authorization: Bearer your_admin_token" http://localhost:28085/admin/clear-dead
```

#### 查看最近访问记录

```http
GET /admin/recent
```

```bash
curl -H "Authorization: Bearer your_admin_token" "http://localhost:28085/admin/recent?limit=20"
```

### 5. 常见状态码

| 状态码 | 含义 |
|---|---|
| 200 | 请求成功 |
| 302 | 重定向成功 |
| 400 | 参数错误；或 `source: "local"` 时请求了 `json` / `text` |
| 401 | 鉴权失败 |
| 403 | Referer 被拦截 |
| 429 | 超过限流额度 |
| 500 | 服务内部错误 |
| 503 | 图池为空，或本地图源的文件已全部缺失 |

## 部署方式

- 二进制部署：见 [docs/systemd.md](docs/systemd.md)
- Docker 部署：见 [docs/docker.md](docs/docker.md)
- 构建与发布：见 [docs/built.md](docs/built.md)
- 开发调试：见 [docs/development.md](docs/development.md)

## 维护与排障

- 访问 /health 可检查服务状态
- 使用 /admin/* 接口进行管理和排障
- 运行日志会输出到标准输出，可直接查看启动、重载和请求信息
- 启动时会检查两个方向的池子，任一方向为空会打印警告并指出该检查 `urls.*` 还是 `local.*_dirs`

## 许可证

本项目采用 Apache License 2.0。详情请参见 [LICENSE](LICENSE)。