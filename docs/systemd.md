# systemd 部署

## 1. 适用场景

适用于 Linux 服务器上长期运行服务，并希望通过 systemd 实现开机自启、自动重启和日志管理的场景。

## 2. 前置条件

- Linux 服务器
- root 或 sudo 权限
- 已编译好二进制文件，或已下载对应 Release 产物

## 3. 部署目录

推荐将服务文件统一放在 `/opt/image-api/`：

```text
/opt/image-api/
├── image-api
├── config.yaml
└── urls/
    ├── portrait.txt
    └── landscape.txt
```

## 4. 准备文件

```bash
sudo mkdir -p /opt/image-api/urls
sudo cp image-api /opt/image-api/
sudo chmod +x /opt/image-api/image-api
sudo cp config.yaml /opt/image-api/
sudo cp urls/*.txt /opt/image-api/urls/
```

## 5. 创建 systemd 服务

创建服务文件：

```bash
sudo tee /etc/systemd/system/image-api.service >/dev/null <<'EOF'
[Unit]
Description=Image API
After=network.target

[Service]
Type=simple
WorkingDirectory=/opt/image-api
ExecStart=/opt/image-api/image-api
Restart=on-failure
RestartSec=5s

[Install]
WantedBy=multi-user.target
EOF
```

## 6. 管理命令

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now image-api
sudo systemctl status image-api
```

### 常用操作

```bash
sudo systemctl start image-api
sudo systemctl stop image-api
sudo systemctl restart image-api
sudo systemctl disable image-api
```

## 7. 日志查看

```bash
sudo journalctl -u image-api -f
sudo journalctl -u image-api -n 100
```

## 8. 常见问题

### 服务启动失败

```bash
sudo systemctl status image-api
sudo journalctl -u image-api -n 50 --no-pager
```

常见原因包括：

- 可执行文件路径错误
- WorkingDirectory 不存在
- 配置文件格式错误
- 端口冲突

### 端口冲突

```bash
sudo ss -tlnp | grep 28085
```

### 权限问题

```bash
sudo chmod +x /opt/image-api/image-api
```

## 9. 进阶配置

可根据需要在 service 文件中追加：

```ini
[Service]
User=image-api
Group=image-api
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
ReadWritePaths=/opt/image-api
```

`source: local` 时，请确保运行服务的用户对 `local.portrait_dirs` / `local.landscape_dirs` 指向的目录有读权限，文件由服务直接读取并返回。

如需刷新 URL 池（包括重新扫描本地图片目录），可执行：

```bash
curl -X POST http://127.0.0.1:28085/admin/reload
```

如果配置了 `auth.admin_token` 或开启了鉴权，请带上 Authorization 头。

## 10. 日志格式

服务日志输出到标准输出，由 journald 统一收集。请求日志为结构化 JSON（含 IP、UA、查询参数），token 参数值已自动脱敏为 `***`，可安全留存或接入日志采集。
