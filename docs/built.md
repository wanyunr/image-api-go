## 使用教程

### 一、前置条件

1. 项目已推送到 GitHub 仓库
2. `.github/workflows/release.yml` 文件已在仓库中
3. 仓库 Settings → Actions → General → Workflow permissions 选择 **Read and write permissions**

### 二、触发方式

Workflow 通过 **推送 Git Tag** 触发，tag 格式为 `v*`（如 `v1.0.0`、`v2.1.3`）。

```
# 1. 确保代码已提交
git add .
git commit -m "release: v1.0.0"
git push origin main

# 2. 打 tag 并推送（触发构建）
git tag v1.0.0
git push origin v1.0.0
```

推送 tag 后，GitHub Actions 自动开始构建。

### 三、构建流程

```text
push tag v1.0.0
    ↓
┌─────────────────────────────────────────┐
│  build job（5 个平台并行编译）            │
│  • linux-amd64                          │
│  • linux-arm64                          │
│  • windows-amd64                        │
│  • darwin-amd64 (Intel Mac)             │
│  • darwin-arm64 (Apple Silicon)         │
└─────────────────────────────────────────┘
    ↓
┌─────────────────────────────────────────┐
│  release job                            │
│  • 下载所有产物                          │
│  • 生成 SHA256 校验文件                  │
│  • 创建 GitHub Release + 自动生成日志    │
└─────────────────────────────────────────┘
```

### 四、产物说明

构建完成后，在仓库的 **Releases** 页面会看到：

| 文件 | 说明                            |
| ------ | --------------------------------- |
| `image-api-linux-amd64`     | Linux x86\_64（服务器常用）  |
| `image-api-linux-arm64`     | Linux ARM64（树莓派/ARM服务器） |
| `image-api-windows-amd64.exe`     | Windows x86\_64              |
| `image-api-darwin-amd64`     | macOS Intel                     |
| `image-api-darwin-arm64`     | macOS Apple Silicon             |
| `checksums-sha256.txt`     | SHA256 校验和                   |

### 五、部署示例

```
# Linux 服务器下载部署
wget https://github.com/你的用户名/image-api/releases/download/v1.0.0/image-api-linux-amd64
chmod +x image-api-linux-amd64
mv image-api-linux-amd64 /usr/local/bin/image-api

# 准备配置和 URL 文件
mkdir -p /opt/image-api/urls
cp config.yaml /opt/image-api/
cp urls/*.txt /opt/image-api/urls/

# 启动
cd /opt/image-api
image-api

# 查看版本
image-api --version
# 输出: image-api v1.0.0
```

### 六、版本号注入

构建时通过 `-ldflags` 将 tag 名注入二进制文件：

```text
# 在 src 目录内构建，产物输出到项目根目录
cd src
go build -ldflags="-s -w -X main.version=v1.0.0" -o ../image-api .

# 本地验证版本号已注入
../image-api --version
# 输出: image-api v1.0.0
```

- 启动日志会显示：`[启动] image-api v1.0.0 | HTTP Server 监听 0.0.0.0:28085`
- 运行 `image-api --version` 或 `image-api -v` 可查看版本号
- 本地 `go build` 不注入时默认显示 `dev`

### 七、常用操作

```
# 发布新版本
git tag v1.1.0
git push origin v1.1.0

# 删除 tag 重新发布（取消构建后）
git tag -d v1.1.0
git push origin :refs/tags/v1.1.0
# 去 GitHub Releases 页面手动删除对应 Release
# 重新打 tag
git tag v1.1.0
git push origin v1.1.0

# 查看 Actions 运行状态
# GitHub 仓库 → Actions 标签页
```

### 八、注意事项

- `CGO_ENABLED=0` 确保纯静态编译，无系统依赖，拷贝即用
- `-s -w` 去掉调试符号，减小二进制体积
- 构建矩阵并行执行，5 个平台总耗时约 1-2 分钟
- Release Notes 由 GitHub 根据 commit 记录自动生成