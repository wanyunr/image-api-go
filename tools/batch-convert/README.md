# batch_convert — 配置化批量图片转换

两行 YAML 配置，把整个目录（含子目录）的常见图片批量转为 WebP。支持多路径、多任务、多进程并行、断点续传与插件扩展。

## 快速开始

```bash
cd tools/batch-convert
pip install -r requirements.txt   # 需 Python 3.9+
pip install pyyaml                # YAML 配置必需

cp examples/config.yaml my_config.yaml   # 复制示例，改成你的目录
python batch_convert.py -c my_config.yaml   # 运行（单/多路径都是这一条命令）
```

- 配置里的路径用正斜杠 `C:/...`
- 源目录必须已存在；输出目录不存在会自动创建

## 配置

### 单路径（最常用）

```yaml
source: C:/你的源目录
target: C:/你的输出目录
```

### 多路径（多组 source/target 一一对应）

```yaml
source:
  - C:/你的源目录1
  - C:/你的源目录2
target:
  - C:/你的输出目录1
  - C:/你的输出目录2
tier: photo        # 公共参数对全部组生效
```

`source` 第 N 项对应 `target` 第 N 项。不写 `target` 时，每个源自动输出到同级 `<源目录名>_converted`。

### 多任务（每组独立参数或不同转换器）

```yaml
tasks:
  - name: 任务1
    source: C:/你的源目录1
    target: C:/你的输出目录1
    tier: photo

  - name: 任务2
    converter: png2jpg
    source: C:/你的源目录2
    target: C:/你的输出目录2
    include: ["*.png"]
```

多组 source/target 但每组参数不同，或需要混合转换器时使用 `tasks` 数组。每组可独立指定下方全部参数。

### 参数一览

| 参数 | 说明 | 默认 |
| --- | --- | --- |
| `source` / `target` | 源目录 / 输出目录（可单值或列表） | 必填；target 缺省自动命名 |
| `converter` | 转换器名称 | `image_webp` |
| `tier` | 质量预设：draft / balanced / photo / high / lossless | `balanced` |
| `quality` | 画质 0-100 | 75 |
| `method` | 压缩努力度 0-6 | 4 |
| `lossless` | 无损模式 | false |
| `alpha_quality` | 透明区域画质 0-100（image_webp） | 跟随 quality |
| `recursive` | 递归处理子目录 | true |
| `skip_existing` | 目标已存在则跳过（断点续传） | true |
| `retries` | 失败重试次数 | 2 |
| `flat` | 输出平铺到目标根目录（不保留子目录结构） | false |
| `include` / `exclude` | 文件过滤通配符列表 | 默认只处理图片格式 |

过滤规则：通配符匹配相对源目录的完整路径（含子目录），如 `"sub/*.jpg"` 只处理 `sub/` 下的 jpg。不写 `include` 时默认处理所有图片格式（PNG/JPG/JPEG/BMP/TIFF/GIF/WebP，WebP 直接拷贝）；写 `["*"]` 处理全部文件。

## 转换器与质量

| 转换器 | 说明 | 主要参数 |
| --- | --- | --- |
| `image_webp`（默认） | 图片转 WebP（GIF 取首帧、透明通道保留、EXIF 方向自动修正） | tier / quality / method / lossless / alpha_quality |
| `copy` | 原样复制文件 | 无 |

```bash
python batch_convert.py --list-converters   # 查看全部转换器（含插件）
```

image_webp 质量权衡：

- **tier 预设**：`draft`(50) → `balanced`(75) → `photo`(82) → `high`(90) → `lossless`(无损)
- **细粒度**：`quality` 越高画质越好、体积越大；`method` 越大压缩越充分、耗时越长；`alpha_quality` 单独控制透明区域

## 并行与可靠性

**并行**：默认按 CPU 核数多进程并行（图片转换是 CPU 密集型）。`--workers N` 调整，`1` 为串行。按文件分发，任务内并行、任务间顺序执行；并行下日志按完成顺序输出。

```bash
python batch_convert.py -c my_config.yaml --workers 8
```

**断点续传**：`skip_existing` 默认开启，目标文件已存在即跳过；中断后重跑同一配置自动续传。

**失败重试**：单文件失败自动重试 `retries` 次（间隔 0.5s/1s 递增），重试前清理半成品；仍失败计入清单，任务继续。

**日志与退出码**：逐文件日志 + 任务/全局两级统计（成功/跳过/失败），失败文件逐条列出。退出码：`0` 全部成功，`1` 有失败，`2` 配置或环境错误。

## 命令行参数

| 参数 | 说明 |
| --- | --- |
| `-c, --config FILE` | 配置文件（.yaml/.yml/.json） |
| `--workers N` | 并行进程数（默认 CPU 核数；1 为串行） |
| `--plugin PATH` | 外部插件文件或目录，可多次指定 |
| `--log-file FILE` | 日志同时写入文件 |
| `--verbose` | DEBUG 级别日志 |
| `--list-converters` | 列出可用转换器后退出 |

## 插件开发

三步定义一个转换器（完整示例见 `examples/plugin_png2jpg.py`）：

```python
from batch_convert import Converter, register

@register
class PngToJpgConverter(Converter):
    name = "png2jpg"

    def output_name(self, src, params) -> str:
        return src.stem + ".jpg"

    def convert(self, src, dst, params) -> None:
        ...   # 读 src 写 dst；失败抛异常即可（自动重试）
```

```bash
python batch_convert.py -c config.yaml --plugin ./my_plugin.py
python batch_convert.py -c config.yaml --plugin ./plugins_dir   # 目录内所有 .py
```

## 与 img2webp 的关系

`img2webp` 是零配置的单目录命令行工具；`batch_convert` 是它的配置化版本（多路径、多任务、插件、并行）。临时转单个目录用 `img2webp`，其余场景用 `batch_convert`。
