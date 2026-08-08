# 开发准则

所有贡献者必须遵守。准则目的是保持项目核心定位不变。

## 1. 核心定位（不可改变）

| 原则 | 说明 |
|------|------|
| Go 标准库优先 | 仅用 `net/http`、`sync`、`crypto/rand` 等标准库；外部依赖仅限 YAML 解析、图片解码 |
| 轻量 | 单二进制部署，无外部服务依赖 |
| 无数据库 | URL 池常驻内存，不引入任何持久化存储 |
| 图片本体不经过 API | 服务仅返回 302，图片流量直达 CDN/OSS |
| 简单直接 | 避免过度设计，安全与性能方案选最简实现 |

## 2. 职责边界

**API 只负责：**

| 职责 | 说明 |
|------|------|
| 选择图片 | 内存 URL 池随机选取，支持 portrait / landscape |
| 鉴权 | Token 校验（Header / Query） |
| 限流 | 全局额度（分/时/天三级） |
| 302 | 重定向到图片 URL |
| metadata | JSON 返回图片元数据（URL/尺寸/格式/文件名） |
| 管理 | 热重载、健康检查、Prometheus 指标 |

**API 不负责：**

| 职责 | 原因 |
|------|------|
| 图片代理 | 流量直达 CDN/OSS，不经过 Go 服务 |
| 图片缓存 | 由 CDN / 浏览器负责 |
| 图片缩放 | 由 CDN / 图片处理服务负责 |
| 图片存储 | 由 OSS / R2 / 对象存储负责 |

> 任何让 API 承担上述"不负责"职责的建议，一律不予采纳。

## 3. 禁止引入的技术

| 类别 | 禁止项 | 原因 |
|------|--------|------|
| Web 框架 | Gin / Echo / Fiber / Chi | `net/http` + `http.ServeMux` 已满足全部路由需求 |
| ORM | GORM / ent / sqlx | 无数据库 |
| 数据库 | PostgreSQL / MySQL / SQLite | URL 池在内存，配置在 YAML |
| 缓存 | Redis / Memcached | 限流用内存计数器，URL 池在内存 |
| 消息队列 | Kafka / RabbitMQ / NATS | 单进程同步处理 |
| 架构模式 | 微服务拆分 / gRPC | 单进程单二进制是核心优势 |

## 4. 架构约束

```
Client → Go HTTP API → 302 → External CDN/OSS
                │
                ├── Auth (Token)
                ├── Rate Limit (全局额度)
                ├── Referer Check (防盗链)
                └── URL Pool (内存常驻)
```

- **限流**：全局计数器（分/时/天三窗口），不按 IP 分桶
- **客户端 IP**：仅用于日志展示，不参与限流
- **热重载**：`POST /admin/reload` 刷新内存 URL 池
- **优雅关闭**：监听 SIGINT/SIGTERM，10s 超时关闭
- **HTTP 超时**：ReadHeader 5s / Read 15s / Write 20s / Idle 60s

## 5. 代码规范

- 模块根目录为 `src/`，`go build` / `go test` / `go vet` 必须在 `src/` 内执行
- 运行时路径（config.yaml、urls/*.txt）相对当前工作目录，推荐在根目录运行
- 文档放 `docs/`，不散落 `.md` 在根目录
- 尽量少子文件夹，业务逻辑统一在 `src/app/`
- 构建用 `CGO_ENABLED=0` 纯静态
- 新增代码**不写大段注释**，需要说明处加一个简短注释

## 6. 改善原则

改善应在**不违反上述约束**的前提下进行，优先方向：

1. **安全性** — Token 比对用 `subtle.ConstantTimeCompare`、日志脱敏
2. **性能** — 连接复用、Range 请求探测元数据、避免不必要分配
3. **可靠性** — 优雅关闭、URL 校验、空池保护
4. **可观测性** — Prometheus 指标、结构化日志
5. **可维护性** — 测试覆盖、配置校验
6. **部署能力** — Docker 多阶段构建、systemd、CI/CD 多平台编译

需要引入第 3 节禁止技术、或让 API 承担第 2 节"不负责"职责的建议，不予采纳。
