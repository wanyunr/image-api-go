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

通过 Git Tag 触发发布流程，推荐使用 `v*` 格式，例如 `v1.0.0`。

```bash
git add .
git commit -m "release: v1.0.0"
git push origin main
git tag v1.0.0
git push origin v1.0.0
```

### 构建产物

发布时通常会生成以下文件：

| 文件 | 说明 |
|---|---|
| image-api-linux-amd64 | Linux x86_64 |
| image-api-linux-arm64 | Linux ARM64 |
| image-api-windows-amd64.exe | Windows x86_64 |
| image-api-darwin-amd64 | macOS Intel |
| image-api-darwin-arm64 | macOS Apple Silicon |
| checksums-sha256.txt | SHA256 校验文件 |

## 4. 构建说明

- 建议使用 `CGO_ENABLED=0` 进行静态编译
- `-s -w` 可减小二进制体积
- Release 页面会自动生成说明与校验文件

## 5. 注意事项

- 若需要在容器中使用，请参考 [docker.md](docker.md)
- 若需要在 Linux 服务中长期运行，请参考 [systemd.md](systemd.md)
- 构建前请确认版本号与发布 tag 一致