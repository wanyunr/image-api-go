# img2webp — 批量图片转 WebP 工具

基于 Python + Pillow 的批量图片格式转换工具，将 **PNG / JPG / JPEG / BMP / TIFF / GIF** 等常见格式统一转换为 **WebP**。

- ✅ 批量处理：遍历输入目录下所有图片，输出到指定目录
- ✅ 递归子目录：自动处理嵌套目录并**保留目录结构**
- ✅ 画质与体积可调：内置质量分级预设 + 细粒度参数
- ✅ 健壮性：损坏文件、非图片文件、转换失败均被捕获并记录日志，不影响整体流程
- ✅ 输出目录不存在时自动创建

## 环境要求

- Python 3.9+
- Pillow >= 10.0（`pip install -r requirements.txt`）

## 安装

```bash
# 进入工具目录
cd tools/img2webp

# 可选：创建虚拟环境（推荐）
python -m venv .venv
# Windows
.venv\Scripts\activate
# Linux / macOS
source .venv/bin/activate

# 安装依赖
pip install -r requirements.txt
```

## 快速开始

```bash
# 将 ./photos 下所有图片（含子目录）转成 WebP 输出到 ./photos_webp
python img2webp.py -i ./photos

# 指定输出目录
python img2webp.py -i ./photos -o ./webp_output

# 不递归，只处理顶层文件
python img2webp.py -i ./photos -o ./webp_output --no-recursive
```

## 命令行参数

| 参数 | 说明 | 默认值 |
| --- | --- | --- |
| `-i, --input DIR` | 输入目录（必填） | — |
| `-o, --output DIR` | 输出目录 | `<输入目录名>_webp`（与输入同级） |
| `--recursive / --no-recursive` | 是否递归处理子目录并保留目录结构 | 开启 |
| `--tier NAME` | 质量分级预设（见下表），使用后忽略 `-q/-m/--lossless` | 不使用 |
| `-q, --quality 0-100` | WebP 画质 | 75 |
| `-m, --method 0-6` | WebP 压缩努力度，越大压缩越充分、耗时越长 | 4 |
| `--lossless` | 无损模式（体积明显更大，适合图标/截图） | 关闭 |
| `--alpha-quality 0-100` | 透明区域（alpha 通道）画质 | 跟随 `--quality` |
| `--keep-original / --no-keep-original` | 转换成功后是否保留原图 | 保留 |
| `--overwrite / --no-overwrite` | 目标文件已存在时是否覆盖 | 跳过 |
| `--ext EXT` | 追加额外输入扩展名，如 `--ext webp --ext avif`（可多次） | 无 |
| `--verbose` | 输出 DEBUG 级详细日志 | 关闭 |
| `--log-file FILE` | 除终端外将日志写入文件 | 无 |
| `-h, --help` | 查看帮助 | — |

## 质量与体积的权衡

WebP 的最终体积由两个维度共同决定，本工具通过**质量分级预设 + 细粒度参数**两种方式开放控制：

### 1. 质量分级预设（`--tier`）

| 预设 | quality | method | lossless | 适用场景 |
| --- | --- | --- | --- | --- |
| `draft` | 50 | 6 | 否 | 预览图、缩略图，极致小体积 |
| `balanced` | 75 | 4 | 否 | **默认档**，官方推荐的"视觉无损"区，通用 |
| `photo` | 82 | 4 | 否 | 照片，视觉几乎无损、体积仍较小 |
| `high` | 90 | 5 | 否 | 高清展示、印刷前检查 |
| `lossless` | 100 | 4 | **是** | 图标、截图、线稿等锐利图像，保证零画质损失 |

```bash
python img2webp.py -i ./photos -o ./out --tier photo
```

### 2. 细粒度自定义

```bash
# 高质量：quality 85 + 更高压缩努力度
python img2webp.py -i ./photos -o ./out -q 85 -m 5

# 小体积优先：quality 60，压缩拉到最满
python img2webp.py -i ./photos -o ./out -q 60 -m 6

# 无损 + 限制透明区域画质
python img2webp.py -i ./photos -o ./out --lossless --alpha-quality 90
```

> **建议**：日常使用 `--tier photo` 或默认 `balanced`；对 PNG 图标/截图类素材用 `--tier lossless`；
> 对画质有苛刻要求的照片用 `-q 90`。转换完成后日志会报告体积节省比例，可据此微调。

## 行为说明

- **目录结构**：`in/a/b.jpg` → `out/a/b.webp`，子目录自动重建。
- **GIF**：取第一帧转为静态 WebP（如需动画请先用其他工具拆帧）。
- **透明图**：PNG/GIF 透明区域保留为 WebP alpha 通道，可用 `--alpha-quality` 单独控制。
- **EXIF 方向**：自动按 EXIF 信息修正旋转/镜像，避免手机照片方向错乱。
- **输出目录在输入目录内**：允许，但扫描时会自动排除输出目录子树，避免重复处理。
- **重复运行**：目标 `.webp` 已存在时默认跳过，加 `--overwrite` 强制覆盖。
- **删除原图**：确认无误后可用 `--no-keep-original`，转换成功的源文件会被删除（失败的文件不会被删）。

## 日志与退出码

- 每个文件转换结果、跳过原因、失败原因都会输出到终端（`--log-file` 可同时落盘）。
- 退出码：`0` 全部成功（或无可转换文件）；`1` 有文件失败；`2` 参数/环境错误（如不支持 WebP）。

## 常见问题

**Q: 报错 "Pillow 不支持 WebP 编码"？**
A: 请升级 Pillow：`pip install -U Pillow`。官方 wheel 已内置 WebP 支持，一般无需手动编译。

**Q: 想把已经转好的 webp 再统一重转/降质？**
A: 追加 `--ext webp` 即可把 webp 也纳入输入（注意搭配 `--overwrite` 使用）。

**Q: 转换失败的文件会中断任务吗？**
A: 不会。损坏/异常文件会被跳过并记录，其余文件继续处理，结束时输出失败清单。
