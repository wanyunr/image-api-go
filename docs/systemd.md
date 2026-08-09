## 使用教程

### 一、前置条件

1. 已按 [built.md](./built.md) 完成编译，得到 `image-api` 二进制文件（或从 GitHub Releases 下载对应平台产物）
2. Linux 服务器（systemd 是 CentOS 7+ / Ubuntu 16.04+ / Debian 8+ 等主流发行版的默认 init 系统）
3. root 或 sudo 权限

### 二、部署目录结构

推荐将程序文件统一放置在 `/opt/image-api/` 下：

```
/opt/image-api/
├── image-api              # 二进制文件
├── config.yaml            # 配置文件
└── urls/
    ├── portrait.txt       # 竖屏 URL 列表
    └── landscape.txt      # 横屏 URL 列表
```

> 说明：程序运行时，`config.yaml` 与 `urls/*.txt` 的路径均以**当前工作目录**为基准。systemd 服务通过 `WorkingDirectory` 指定工作目录，因此部署目录即工作目录。

部署步骤：

```bash
# 创建目录
sudo mkdir -p /opt/image-api/urls

# 放置二进制（假设已下载或编译好）
sudo cp image-api /opt/image-api/
sudo chmod +x /opt/image-api/image-api

# 放置配置与 URL 列表
sudo cp config.yaml /opt/image-api/
sudo cp urls/*.txt /opt/image-api/urls/

# 验证版本
/opt/image-api/image-api --version
# 输出: image-api v1.0.0
```

### 三、创建 systemd 服务

#### 1. 服务文件存放路径

systemd 服务单元文件的标准存放路径（按优先级从高到低）：

| 路径 | 用途 |
|------|------|
| `/etc/systemd/system/` | **管理员自定义服务（推荐）** |
| `/run/systemd/system/` | 运行时临时服务 |
| `/usr/lib/systemd/system/` | 软件包安装的服务（不建议手动修改） |

本项目服务文件路径：**`/etc/systemd/system/image-api.service`**

#### 2. service 单元文件内容

创建 `/etc/systemd/system/image-api.service`：

```bash
sudo vim /etc/systemd/system/image-api.service
```

写入以下内容：

```ini
[Unit]
Description=Image API - 轻量级随机图片服务
After=network.target

[Service]
Type=simple
WorkingDirectory=/opt/image-api
ExecStart=/opt/image-api/image-api
Restart=on-failure
RestartSec=5s

[Install]
WantedBy=multi-user.target
```

#### 3. 单元文件配置说明

**`[Unit]` 部分 —— 服务描述与启动依赖**

| 字段 | 说明 |
|------|------|
| `Description` | 服务描述，`systemctl status` 时显示 |
| `After` | 在指定目标之后启动（网络就绪后再拉起本服务） |

**`[Service]` 部分 —— 服务运行行为（核心）**

| 字段 | 说明 |
|------|------|
| `Type=simple` | 默认类型，`ExecStart` 启动的进程即主进程，适合前台常驻服务 |
| `WorkingDirectory` | 工作目录，决定 `config.yaml` 与 `urls/` 的相对路径基准 |
| `ExecStart` | 启动命令，即二进制绝对路径 |
| `Restart=on-failure` | 进程异常退出时自动重启（正常退出不重启） |
| `RestartSec=5s` | 重启间隔，避免频繁重启 |

**`[Install]` 部分 —— 开机自启配置**

| 字段 | 说明 |
|------|------|
| `WantedBy=multi-user.target` | 等价于旧版 runlevel 3（多用户命令行），`enable` 时建立到该目标的软链 |

### 四、常用 systemctl 管理命令

```bash
# 修改 service 文件后必须重载 systemd 配置
sudo systemctl daemon-reload

# 启动 / 停止 / 重启
sudo systemctl start image-api
sudo systemctl stop image-api
sudo systemctl restart image-api

# 查看运行状态（含最近日志）
sudo systemctl status image-api

# 开机自启 / 禁用开机自启
sudo systemctl enable image-api
sudo systemctl disable image-api

# 开机自启并立即启动（常用组合）
sudo systemctl enable --now image-api

# 查看是否已设为开机自启
systemctl is-enabled image-api

# 查看服务是否正在运行
systemctl is-active image-api
```

> 提示：修改 `urls/*.txt` 后刷新 URL 池，可直接 `curl http://127.0.0.1:28085/admin/reload`（无需重启服务）；或简单地 `systemctl restart image-api`。

### 五、服务日志查看

systemd 会自动捕获服务进程输出到标准输出/标准错误的日志（本服务日志全部输出到 stdout），通过 `journalctl` 查看：

```bash
# 查看全部日志
sudo journalctl -u image-api

# 实时跟踪日志（类似 tail -f，Ctrl+C 退出）
sudo journalctl -u image-api -f

# 最近 100 行
sudo journalctl -u image-api -n 100

# 今日日志
sudo journalctl -u image-api --since today

# 指定时间范围
sudo journalctl -u image-api --since "2026-08-02 20:00" --until "2026-08-02 22:00"

# 只看错误及以上级别（emerg/alert/crit/err）
sudo journalctl -u image-api -p err

# 查看本次启动以来的日志
sudo journalctl -u image-api -b

# 查看上一次启动的日志（服务崩溃后排查）
sudo journalctl -u image-api -b -1
```

> 提示：本服务启动日志包含配置加载、URL 池数量、监听地址与各功能开关；每个请求会打印一行 `[request] IP=... UA=... Query=...`（`Query` 中的 `token` 参数值已自动脱敏为 `***`），可直接在日志中观察访问情况。

### 六、常见问题排查

#### 1. 服务启动失败

```bash
# 查看详细错误
sudo systemctl status image-api
sudo journalctl -u image-api -n 50 --no-pager
```

常见原因：

- **`ExecStart` 路径错误**：确认二进制绝对路径正确且有可执行权限（`ls -l /opt/image-api/image-api`）
- **`WorkingDirectory` 不存在**：确认目录已创建，且 `config.yaml`、`urls/` 在该目录下
- **配置校验失败**：日志会提示具体字段，如 `server.port 必须在 1-65535 之间`
- **端口被占用**：见下条

#### 2. 端口被占用

```bash
# 查看端口占用
sudo ss -tlnp | grep 28085
# 或
sudo lsof -i :28085
```

解决方法：停止占用进程，或在 `config.yaml` 中修改 `server.port` 后 `systemctl restart image-api`。

#### 3. 权限问题

- **二进制无执行权限**：`sudo chmod +x /opt/image-api/image-api`
- **目录无访问权限**：确认 `WorkingDirectory` 及其下文件可被运行用户读取（默认以 root 运行，一般无此问题；若改用非 root 用户见第七节）

#### 4. 修改 service 文件后不生效

```bash
# 任何对 .service 文件的修改都必须先重载
sudo systemctl daemon-reload
sudo systemctl restart image-api
```

#### 5. 服务频繁重启

- 查看 `Restart=on-failure` 与 `RestartSec=5s` 配置是否合理
- 用 `journalctl -u image-api -b -1` 查看崩溃前的日志定位根因
- 临时关闭自动重启以便调试：`sudo systemctl edit image-api`，写入
  ```ini
  [Service]
  Restart=no
  ```
  然后 `daemon-reload && restart`，调试完 `systemctl revert image-api` 恢复

#### 6. 开机自启未生效

```bash
# 确认已 enable
systemctl is-enabled image-api
# 应输出 enabled

# 确认软链存在
ls -l /etc/systemd/system/multi-user.target.wants/image-api.service
```

若为 `disabled`，执行 `sudo systemctl enable image-api`。

### 七、进阶配置（可选）

最小配置足以运行。生产环境可按需追加以下选项到 `[Service]` 段：

#### 0. metadata 获取模式

`config.yaml` 中 `server.metadata_mode` 可选：
- `on_request`（默认）：请求 `format=json` 时实时探测，无缓存零启动时间
- `prefetch`：启动时从缓存快速入池，后台异步补抓全量 metadata

#### 1. 使用非 root 用户运行

```bash
# 创建专用用户
sudo useradd -r -s /sbin/nologin -d /opt/image-api image-api
sudo chown -R image-api:image-api /opt/image-api
```

```ini
[Service]
User=image-api
Group=image-api
```

#### 2. 安全加固

```ini
[Service]
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
ReadWritePaths=/opt/image-api
```

- `NoNewPrivileges`：禁止服务及子进程获取新权限
- `ProtectSystem=strict`：只读保护 `/`，仅 `ReadWritePaths` 指定目录可写
- `ProtectHome`：隔离 `/home`、`/root`、`/run/user`
- `PrivateTmp`：私有 `/tmp`，与其他服务隔离
- `ReadWritePaths`：在 `ProtectSystem=strict` 下显式放行可写目录

#### 3. 热重载（`systemctl reload` 触发 URL 池刷新）

```ini
[Service]
ExecReload=/usr/bin/curl -fsS http://127.0.0.1:28085/admin/reload
```

配置后即可用 `sudo systemctl reload image-api` 刷新 URL 池（不中断服务）。若开启了鉴权（`auth.enabled: true`），需补充 `-H "Authorization: Bearer <token>"`。

#### 4. 资源限制

```ini
[Service]
LimitNOFILE=65536      # 文件描述符上限（高并发场景）
MemoryMax=512M         # 内存上限
CPUQuota=100%          # CPU 配额
```

#### 5. 其他建议

- **防火墙**：仅开放服务端口（如 28085），避免直接暴露管理接口 `/admin/reload`
- **反向代理**：生产环境建议前置 Nginx/Caddy 处理 TLS、限流、缓存，本服务仅监听 `127.0.0.1`；若以子目录路径反代（如 `example.com/api/`），在 `config.yaml` 中配置 `server.base_path`（详见 README「反代子目录部署」）
- **配置外部化**：将 `config.yaml` 与 `urls/` 放在部署目录，二进制单独管理，便于版本升级时保留配置
