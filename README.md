# Image API

[![Release](https://img.shields.io/github/v/release/wanyunr/image-api-go)](https://github.com/wanyunr/image-api-go/releases/latest)

Image API 是一个轻量级随机图片服务，基于 Go 标准库和内存 URL 池构建。默认不代理图片流量，只返回一条随机图片地址，适合接入 CDN、OSS 或对象存储；也可以启用本机图片目录，由服务直接返回文件。

### 核心能力

- 默认返回 302 跳转到外部图片 URL
- 支持 portrait / landscape 两种方向
- 支持 JSON、TEXT、REDIRECT 三种输出格式（`source: "local"` 时仅支持 REDIRECT）
- 支持本机图片目录作为图源（直接返回文件）
- 支持 Token 鉴权、全局限流、防盗链
- 支持热重载、健康检查、最近访问追溯

## 快速开始

以下三种方式任选其一。首次启动时若配置文件或 URL 列表不存在，程序会自动生成带注释的默认文件（`config.yaml`、`urls/portrait.txt`、`urls/landscape.txt`），修改后重启即可。

### 方式一：Docker 镜像（推荐）

镜像发布在 GHCR，双架构（amd64/arm64），`latest` 与版本号标签（如 `v1.3.0`）均可使用：

```bash
docker run -d --name image-api \
  -p 28085:28085 \
  -v "$PWD/config.yaml:/app/config.yaml:ro" \
  -v "$PWD/urls:/app/urls:ro" \
  ghcr.io/wanyunr/image-api-go:latest
```

完整的 compose 用法（含缓存与本地图源挂载）见 [docs/docker.md](docs/docker.md)。

### 方式二：下载 Release 二进制

从 [Releases](https://github.com/wanyunr/image-api-go/releases/latest) 下载对应平台的可执行文件（linux/darwin/windows × amd64/arm64），校验 `checksums-sha256.txt` 后直接运行：

```bash
chmod +x image-api-linux-amd64
./image-api-linux-amd64
```

### 方式三：源码构建

需要 Go 1.25+。Go 模块根目录是 `src/`，建议在项目根目录运行生成的二进制，配置与 URL 列表路径才能按预期解析：

```bash
cd src
go build -o ../image-api .
cd ..
./image-api
```

### 验证服务

```bash
curl http://127.0.0.1:28085/health
curl -i http://127.0.0.1:28085/
```

## 配置

完整带注释的配置模板见 [config.yaml.example](config.yaml.example)（复制为 `config.yaml` 使用）。速查表：

| 配置项 | 默认值 | 说明 |
|---|---|---|
| `source` | `url` | 图片来源：`url`（URL 列表）或 `local`（本机目录）；未声明时按 `url` 处理，兼容旧配置 |
| `server.host` | `0.0.0.0` | 监听地址 |
| `server.port` | `28085` | HTTP 监听端口 |
| `server.base_path` | `/` | 反向代理子路径前缀，如 `/api` 后路由变为 `/api/`、`/api/health` |
| `server.metadata_mode` | `on_request` | `on_request`：请求时实时探测，启动更快；`prefetch`：启动预取 + 后台补抓，适合大池 |
| `server.cache_dir` | `cache` | metadata 缓存目录，仅 prefetch 模式使用 |
| `server.recent_buffer_size` | `50` | 最近访问记录条数（`/admin/recent`），重启后清空 |
| `urls.portrait` / `urls.landscape` | `urls/*.txt` | 竖屏/横屏 URL 列表文件，仅 `source: "url"` 时生效 |
| `local.extensions` | jpg/jpeg/png/webp/gif | 参与扫描的扩展名，不区分大小写 |
| `local.portrait_dirs` / `local.landscape_dirs` | `images/*` | 图片目录，仅 `source: "local"` 时生效；启动与 `/admin/reload` 时扫描 |
| `auth.enabled` | `false` | Token 鉴权开关 |
| `auth.tokens` | — | 普通接口可用的 Token 列表 |
| `auth.admin_token` | 空 | 管理端点专用 Token，优先级高于 `auth.tokens` |
| `rate_limit.enabled` | `false` | 全局限流开关（按全服务合计计数，不按 IP 分桶） |
| `rate_limit.per_minute` / `per_hour` / `per_day` | 30 / 120 / 300 | 各窗口最大请求数，0 表示不限 |
| `referer.enabled` | `false` | Referer 防盗链开关 |
| `referer.allow_empty` | `true` | 是否允许空 Referer |
| `referer.whitelist` | — | 白名单，支持精确匹配与 `*.example.com` 通配符 |

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

# 开启鉴权后使用 Token（Authorization 头或 query 参数均可）
curl -H "Authorization: Bearer your_secret_token" "http://localhost:28085/?format=json"
curl "http://localhost:28085/?token=your_secret_token&format=json"
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

仅在 `source: "local"` 时注册。`<id>` 是启动扫描时按文件绝对路径算出的哈希，只有被扫描到的文件才能通过这个路由访问，因此不存在目录穿越风险；目录内的隐藏文件、隐藏子目录与符号链接不会进入白名单，目录路径按服务进程的工作目录解析。

- 随机接口直接返回文件内容，不暴露该地址；它主要出现在 `/admin/recent` 的记录里，便于排查"刚才返回的是哪张图"。
- 同样受 `auth`、`referer`、`rate_limit` 配置约束。
- 文件在扫描后被删除时，随机接口会自动重选；全部文件都缺失时返回 503，`/admin/reload` 可重新扫描。文件在扫描后被替换为符号链接时同样按不可用处理（读取阶段会校验路径仍指向同一普通文件）。
- 启动 → 扫描 → 请求处理 → 异常分支的完整流程图见 [docs/development.md](docs/development.md#本地图源source-local流程)。

### 4. 管理接口

管理接口专门用于刷新池子、清理死链和查看最近访问记录。它们优先使用 `auth.admin_token` 进行鉴权；如果没有配置 `admin_token`，则仅当 `auth.enabled: true` 时才会回退到普通 `auth.tokens` 中的 Token。若 `auth.enabled: false` 且未配置 `admin_token`，管理接口将返回 401。

#### 热重载 URL 池

```http
GET  /admin/reload?token=your_admin_token
POST /admin/reload
```

重新加载 URL 列表（`source: "local"` 时重新扫描图片目录），并刷新内存池。适用于修改了 `urls/*.txt` 或增删了本地图片后立即生效。

- **GET**：可直接在浏览器地址栏访问或用 `<a href>` 收藏，`token` 通过查询参数传递，适合“点一下就刷新”。
- **POST**：适合脚本/CI，`token` 走 `Authorization` 头，避免 token 出现在 URL 与访问日志中。
- 两种方法效果完全一致；其它方法返回 405。

```bash
# 浏览器/命令行直接访问（GET + 查询参数）
curl "http://localhost:28085/admin/reload?token=your_admin_token"

# 脚本调用（POST + 请求头）
curl -X POST -H "Authorization: Bearer your_admin_token" http://localhost:28085/admin/reload

# 如果未配置 admin_token，则使用普通 Token
curl "http://localhost:28085/admin/reload?token=your_secret_token"
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

## 部署与运维

详细文档：

- [docs/systemd.md](docs/systemd.md) —— 二进制 + systemd 部署
- [docs/docker.md](docs/docker.md) —— Docker / docker-compose 部署
- [docs/built.md](docs/built.md) —— 构建与发布流程（tag 推送自动出二进制 Release 与 GHCR 镜像）
- [docs/development.md](docs/development.md) —— 开发指南、代码结构与调试

日常维护：

- 访问 `/health` 检查服务状态
- 使用 `/admin/*` 接口重载池子、清理死链、追溯最近返回的图片
- 运行日志输出到标准输出（结构化 JSON，token 已脱敏），可直接查看启动、重载和请求信息
- 启动时会检查两个方向的池子，任一方向为空会打印警告并指出该检查 `urls.*` 还是 `local.*_dirs`

## 项目结构

```text
image-api/
├── src/                # Go 模块根目录（main.go 与 app/ 业务包）
├── config.yaml.example # 配置模板，复制为 config.yaml 使用
├── urls/               # URL 列表目录（source: "url"）
├── images/             # 本机图片目录（source: "local"）
├── test/               # 端到端测试脚本与配置
├── tools/              # 辅助脚本（批量转换、WebP 转换，各含独立 README）
├── docs/               # 文档目录
├── docker/             # Dockerfile 与 docker-compose.yml
└── LICENSE
```

`src/app/` 内的文件级结构见 [docs/development.md](docs/development.md#代码结构)。

## 许可证

本项目采用 Apache License 2.0。详情请参见 [LICENSE](LICENSE)。
