# Image API

轻量级随机图片 API，使用 Go 标准库构建，零框架、无数据库依赖。

访问 API 即自动 302 重定向到一张随机图片，浏览器直接展示，图片不经过本服务器。

## 项目结构

```
image-api/
├── main.go          # 入口：配置加载、组件初始化、路由注册、启动服务
├── config.go        # 配置结构体定义、加载与校验
├── handler.go       # HTTP 处理器（ServeImage / Health / Reload / 鉴权 / 防盗链）
├── pool.go          # URL 池管理（文件加载、随机选取、热重载）
├── imageinfo.go     # 图片元数据探测（Range 请求 + DecodeConfig）
├── ratelimit.go     # 基于 IP 的滑动窗口限流
├── util.go          # 通用工具函数（IP提取、UA检测、Referer匹配、URL解析、时间格式化）
├── config.yaml      # 配置文件
├── urls/
│   ├── portrait.txt
│   └── landscape.txt
└── logs/
```

## 快速开始

```bash
# 编译
go build -o image-api .

# 运行（默认读取 config.yaml）
./image-api

# 指定配置文件
./image-api /path/to/config.yaml
```

## 配置说明

```yaml
server:
  host: "0.0.0.0"
  port: 8080

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
# 浏览器访问 → 直接看到图片
http://localhost:8080/

# 强制竖屏
http://localhost:8080/?orientation=portrait

# JSON 格式
curl "http://localhost:8080/?format=json"
# {"url":"https://cdn.example.com/p/001.jpg","orientation":"landscape"}

# 纯文本
curl "http://localhost:8080/?format=text"
# https://cdn.example.com/p/001.jpg
```

### 健康检查

```
GET /health
```

```json
{"status":"ok","portrait":3,"landscape":3}
```

### 热重载 URL 列表

```
GET/POST /admin/reload
```

修改 `portrait.txt` / `landscape.txt` 后无需重启，调用此接口即可刷新内存中的 URL 池。

```json
{"status":"reloaded","portrait":5,"landscape":8}
```

## 鉴权

开启 `auth.enabled: true` 后，所有请求需携带 Token：

```bash
# Query 参数方式
curl "http://localhost:8080/?token=your-secret-token"

# Header 方式
curl -H "Authorization: Bearer your-secret-token" http://localhost:8080/
```

未授权返回 `401 Unauthorized`。

## 频率限制

开启 `rate_limit.enabled: true` 后，按客户端 IP 限制访问频率，支持三级限制同时生效：

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
- 图片不经过本服务器，仅返回 302 重定向
- 纯标准库实现，无第三方框架开销

**推荐部署架构：**

```
Client → Cloudflare / Nginx → Go API → 返回 CDN/R2/OSS 图片 URL
```
