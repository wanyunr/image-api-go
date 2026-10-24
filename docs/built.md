# 构建与发布

## 1. 构建目标

本项目支持构建为单文件二进制，可直接部署到 Linux、Windows、macOS 等环境。

## 2. 本地构建

```bash
cd src
go build -o ../image-api .
```

如需注入版本号：

```bash
cd src
go build -ldflags="-s -w -X main.version=v1.0.0" -o ../image-api .
```

验证版本：

```bash
./image-api --version
```

## 3. 发布流程

### 触发方式

通过 Git Tag 触发发布流程，使用 `v*` 格式，例如 `v1.3.0`：

```bash
git push origin main
git tag v1.3.0
git push origin v1.3.0
```

版本号由 CI 自动从标签名注入二进制（`image-api --version` 可验证），无需手工修改代码中的版本常量。

### 构建产物

发布时会生成以下产物：

| 产物 | 说明 |
|---|---|
| image-api-linux-amd64 | Linux x86_64 |
| image-api-linux-arm64 | Linux ARM64 |
| image-api-windows-amd64.exe | Windows x86_64 |
| image-api-darwin-amd64 | macOS Intel |
| image-api-darwin-arm64 | macOS Apple Silicon |
| checksums-sha256.txt | SHA256 校验文件 |
| ghcr.io/wanyunr/image-api-go | 双架构容器镜像（linux/amd64、linux/arm64），推送 `<tag>` 与 `latest` 标签 |

以上均由 `.github/workflows/release.yml` 在 tag 推送后自动完成：二进制与校验文件作为 Release 附件发布，镜像推送到 GHCR（使用自带 `GITHUB_TOKEN` 认证）。手动触发（workflow_dispatch）只验证构建，发布 `dev` 标签镜像，不创建 Release。

## 4. 构建说明

- 建议使用 `CGO_ENABLED=0` 进行静态编译
- `-s -w` 可减小二进制体积
- Release 页面会自动生成说明与校验文件

## 5. 注意事项

- 若需要在容器中使用，请参考 [docker.md](docker.md)
- 若需要在 Linux 服务中长期运行，请参考 [systemd.md](systemd.md)
- 本地手动构建时版本号默认为 `dev`，需要正式版本号时通过 `-ldflags "-X main.version=<tag>"` 注入
- 发布前建议跑通 [development.md](development.md) 中的单元测试与 e2e 套件