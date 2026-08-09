# 开发指南

## 目标与约束

本项目的开发应始终遵循以下原则：

- 以 Go 标准库为主，避免引入重型框架
- 保持单二进制、无数据库、零外部依赖服务的定位
- 图片本体不经过 API 服务，服务只负责选择和返回地址
- 代码实现尽量简单、直接、可维护

## 开发环境

### 前置要求

- Go 1.21 或更高版本
- 终端环境可访问网络（用于拉取依赖）

### 运行目录说明

Go 模块根目录为 src/。因此：

```bash
cd src
go build ./...
go test ./...
```

若从项目根目录直接运行，会导致配置文件和 URL 列表路径解析错误。

## 常用开发命令

```bash
# 在 src/ 目录内执行
cd src

go test ./...
go vet ./...
go test -race ./...
```

## 代码结构

| 目录/文件 | 说明 |
|---|---|
| src/main.go | 服务入口，启动 HTTP Server |
| src/app/handler.go | HTTP 请求处理、路由逻辑 |
| src/app/pool.go | URL 池、metadata 预取与后台补抓 |
| src/app/config.go | 配置加载与校验 |
| src/app/cache.go | metadata 缓存持久化 |
| src/app/ratelimit.go | 全局限流 |
| src/app/imageinfo.go | 图片元数据探测 |
| src/app/recent.go | 最近访问追溯 |

## 调试建议

### 本地调试

```bash
cd src
go build -o ../image-api .
cd ..
./image-api
```

### 关键观察点

- 启动日志：配置加载、URL 池数量、监听地址
- 请求日志：IP、UA、查询参数（Token 已脱敏）
- 管理接口：/health、/admin/reload、/admin/clear-dead

## 测试与质量

建议在修改逻辑后至少执行：

```bash
cd src
go test ./...
go vet ./...
```

## 贡献规范

- 新增功能时优先保持项目轻量与简单
- 不引入 Web 框架、数据库、消息队列等不必要组件
- 代码注释仅在必要处添加，避免冗长说明
- 文档更新应与代码变更同步完成

## 备注

如果新增功能会改变当前设计边界（例如让 API 承担图片代理、存储或缓存职责），应先评估是否违背项目定位。
