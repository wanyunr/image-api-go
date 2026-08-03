#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
img2webp - 批量图片转 WebP 工具

将 PNG / JPG / JPEG / BMP / TIFF / GIF 等常见图片格式批量转换为 WebP，
支持递归处理子目录并保留目录结构，可在画质与体积之间灵活权衡。

用法示例:
    python img2webp.py -i ./photos -o ./photos_webp                # 默认平衡档
    python img2webp.py -i ./photos -o ./photos_webp --tier photo   # 照片预设
    python img2webp.py -i ./photos -o ./photos_webp -q 85 -m 4     # 自定义质量
    python img2webp.py -i ./photos -o ./photos_webp --overwrite --no-keep-original
    python img2webp.py -i ./photos -o ./photos_webp --no-recursive

依赖: Pillow (pip install -r requirements.txt)
"""

from __future__ import annotations

import argparse
import logging
import sys
from dataclasses import dataclass, field
from pathlib import Path
from typing import Optional, Set

from PIL import Image, ImageOps, UnidentifiedImageError, features

# 默认支持的输入扩展名（统一转换为 WebP，不含 .webp，避免重复转换）
SUPPORTED_INPUT_EXTS: Set[str] = {
    ".png", ".jpg", ".jpeg", ".bmp", ".tif", ".tiff", ".gif",
}

# 质量分级预设: 名称 -> (quality, method, lossless)
#   quality  0-100 : WebP 画质，越高画质越好、体积越大
#   method   0-6   : WebP 压缩努力度，越高压缩越充分、耗时越长（0 最快最省 CPU）
#   lossless True  : 启用无损模式（体积明显更大，适合图标、截图等锐利图像）
TIERS = {
    "draft":    (50, 6, False),  # 极致小体积，速度与体积优先
    "balanced": (75, 4, False),  # 体积与画质平衡（默认档，WebP 官方推荐的视觉无损区）
    "photo":    (82, 4, False),  # 照片推荐：视觉几乎无损，体积较小
    "high":     (90, 5, False),  # 高画质，体积较大
    "lossless": (100, 4, True),  # 无损压缩（PNG 类内容效果最好）
}
DEFAULT_TIER = "balanced"


@dataclass
class Stats:
    """转换统计信息。"""
    converted: int = 0
    skipped: int = 0
    failed: int = 0
    total_bytes_in: int = 0
    total_bytes_out: int = 0
    failures: list = field(default_factory=list)


def setup_logging(verbose: bool, log_file: Optional[str]) -> logging.Logger:
    """配置日志：默认 INFO 输出到 stderr，--verbose 输出 DEBUG，可选写入日志文件。"""
    logger = logging.getLogger("img2webp")
    logger.setLevel(logging.DEBUG if verbose else logging.INFO)
    logger.propagate = False
    fmt = logging.Formatter("%(asctime)s [%(levelname)s] %(message)s", "%H:%M:%S")

    handler = logging.StreamHandler(sys.stderr)
    handler.setFormatter(fmt)
    logger.addHandler(handler)

    if log_file:
        fh = logging.FileHandler(log_file, encoding="utf-8")
        fh.setFormatter(fmt)
        logger.addHandler(fh)
    return logger


def iter_images(root: Path, recursive: bool, extra_exts: Set[str],
                exclude: Optional[Path]) -> list[Path]:
    """遍历输入目录下的所有图片文件，可选排除某目录子树（如输出目录）。"""
    exts = SUPPORTED_INPUT_EXTS | {
        e if e.startswith(".") else "." + e for e in extra_exts
    }
    exts = {e.lower() for e in exts}

    pattern = "**/*" if recursive else "*"
    files: list[Path] = []
    for p in root.glob(pattern):
        if not p.is_file():
            continue
        if p.suffix.lower() not in exts:
            continue
        # 输出目录位于输入目录内部时，跳过其子树，避免把生成的 webp 当输入
        if exclude is not None and (p == exclude or exclude in p.parents):
            continue
        files.append(p)
    return files


def convert_image(src: Path, dst: Path, *, quality: int, method: int,
                  lossless: bool, alpha_quality: Optional[int],
                  overwrite: bool, keep_original: bool,
                  log: logging.Logger) -> tuple[str, int, int]:
    """转换单张图片。

    返回 (status, 原大小字节数, 目标大小字节数)，status 为:
        "converted" 转换成功
        "skipped"   目标已存在且未指定覆盖
        "failed"    转换失败
    """
    if dst.exists() and not overwrite:
        log.info("跳过（目标已存在）: %s", dst)
        return "skipped", 0, 0

    try:
        # verify() 校验文件完整性，损坏文件在此抛出 UnidentifiedImageError
        with Image.open(src) as check:
            check.verify()

        # verify 之后必须重新打开才能正常使用
        with Image.open(src) as im:
            im = ImageOps.exif_transpose(im)  # 按 EXIF 修正旋转/镜像，保证方向正确
            im.seek(0)                        # 多帧格式（GIF）只取第一帧

            # 模式归一化：带透明度的转 RGBA，否则转 RGB（WebP 不支持索引色 P 模式）
            if im.mode in ("RGBA", "LA") or (im.mode == "P" and "transparency" in im.info):
                im = im.convert("RGBA")
            else:
                im = im.convert("RGB")

            save_kwargs = {
                "quality": quality,
                "method": method,
                "lossless": lossless,
            }
            if alpha_quality is not None:
                save_kwargs["alpha_quality"] = alpha_quality

            dst.parent.mkdir(parents=True, exist_ok=True)
            im.save(dst, "WEBP", **save_kwargs)

        in_size = src.stat().st_size
        out_size = dst.stat().st_size
        log.info("转换成功: %s -> %s (%d -> %d bytes, %.1f%%)",
                 src, dst, in_size, out_size,
                 (1 - out_size / in_size) * 100 if in_size else 0)
    except UnidentifiedImageError as exc:
        log.warning("无法识别图片（文件损坏或不是图片）: %s (%s)", src, exc)
        return "failed", 0, 0
    except (OSError, ValueError) as exc:
        log.warning("转换失败: %s (%s)", src, exc)
        return "failed", 0, 0

    # 转换成功后按需删除原图（删除失败不影响结果，仅记录警告）
    if not keep_original:
        try:
            src.unlink()
        except OSError as exc:
            log.warning("转换成功但删除原图失败: %s (%s)", src, exc)
    return "converted", in_size, out_size


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="img2webp",
        description="批量将 PNG/JPG/JPEG/BMP/TIFF/GIF 等图片转换为 WebP，"
                    "支持递归处理子目录并保留目录结构。",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog=(
            "示例:\n"
            "  python img2webp.py -i ./photos -o ./photos_webp\n"
            "  python img2webp.py -i ./photos -o ./photos_webp --tier photo\n"
            "  python img2webp.py -i ./photos -o ./photos_webp -q 85 -m 4\n"
            "  python img2webp.py -i ./photos -o ./photos_webp --overwrite --no-keep-original\n"
            "  python img2webp.py -i ./photos -o ./photos_webp --no-recursive\n"
        ),
    )
    parser.add_argument("-i", "--input", required=True,
                        help="输入目录（必填）")
    parser.add_argument("-o", "--output",
                        help="输出目录（默认: 输入目录同级下的 '<输入目录名>_webp'）")
    parser.add_argument("--recursive", action=argparse.BooleanOptionalAction, default=True,
                        help="递归处理子目录并保留目录结构（默认开启；--no-recursive 关闭）")
    parser.add_argument("--tier", choices=sorted(TIERS.keys()),
                        help="质量分级预设: %s（使用预设时忽略 --quality/--method/--lossless）"
                             % ", ".join(sorted(TIERS.keys())))
    parser.add_argument("-q", "--quality", type=int, default=75, metavar="0-100",
                        help="WebP 画质 0-100（默认 75），越高画质越好、体积越大")
    parser.add_argument("-m", "--method", type=int, default=4, choices=range(0, 7),
                        metavar="0-6",
                        help="WebP 压缩努力度 0-6（默认 4），越大压缩越充分、耗时越长")
    parser.add_argument("--lossless", action="store_true",
                        help="使用无损模式（体积明显更大，适合图标/截图等锐利图像）")
    parser.add_argument("--alpha-quality", type=int, default=None, metavar="0-100",
                        help="透明区域（alpha 通道）画质 0-100，默认跟随 --quality")
    parser.add_argument("--keep-original", action=argparse.BooleanOptionalAction, default=True,
                        help="转换后保留原图（默认保留；--no-keep-original 转换成功后删除原图）")
    parser.add_argument("--overwrite", action=argparse.BooleanOptionalAction, default=False,
                        help="覆盖已存在的输出文件（默认跳过已存在文件）")
    parser.add_argument("--ext", action="append", default=[], metavar="EXT",
                        help="追加额外支持的输入扩展名，如 --ext webp --ext avif（可多次使用）")
    parser.add_argument("--verbose", action="store_true",
                        help="输出 DEBUG 级别详细日志")
    parser.add_argument("--log-file", metavar="FILE",
                        help="除终端外，同时将日志写入指定文件")
    return parser


def main(argv: Optional[list[str]] = None) -> int:
    args = build_parser().parse_args(argv)

    # Windows 控制台统一使用 UTF-8，避免中文日志乱码
    if sys.platform == "win32":
        for stream in (sys.stdout, sys.stderr):
            try:
                stream.reconfigure(encoding="utf-8", errors="replace")
            except (AttributeError, ValueError):
                pass

    log = setup_logging(args.verbose, args.log_file)

    # WebP 编码能力检查
    if not features.check("webp"):
        log.error("当前 Pillow 不支持 WebP 编码，请升级: pip install -U Pillow"
                  "（Windows/macOS 官方 wheel 已内置 WebP 支持）")
        return 2

    src_root = Path(args.input).expanduser().resolve()
    if not src_root.is_dir():
        log.error("输入目录不存在或不是目录: %s", src_root)
        return 2

    dst_root = (Path(args.output).expanduser().resolve()
                if args.output
                else src_root.with_name(src_root.name + "_webp"))

    # 质量参数：使用预设则覆盖细粒度参数
    if args.tier:
        quality, method, lossless = TIERS[args.tier]
        log.info("使用质量预设 [%s]: quality=%d, method=%d, lossless=%s",
                 args.tier, quality, method, lossless)
    else:
        quality, method, lossless = args.quality, args.method, args.lossless

    if not 0 <= quality <= 100:
        log.error("--quality 必须在 0-100 之间，收到: %d", quality)
        return 2
    if args.alpha_quality is not None and not 0 <= args.alpha_quality <= 100:
        log.error("--alpha-quality 必须在 0-100 之间，收到: %d", args.alpha_quality)
        return 2

    # 输出目录位于输入目录内部时提示（扫描阶段会自动排除该子树）
    if dst_root == src_root or src_root in dst_root.parents:
        log.warning("输出目录位于输入目录内部: %s（扫描时已自动排除该目录）", dst_root)

    try:
        dst_root.mkdir(parents=True, exist_ok=True)
    except OSError as exc:
        log.error("无法创建输出目录: %s (%s)", dst_root, exc)
        return 2

    files = iter_images(src_root, args.recursive, set(args.ext), dst_root)
    log.info("输入目录: %s", src_root)
    log.info("输出目录: %s", dst_root)
    log.info("共发现 %d 个待转换文件", len(files))
    if not files:
        log.info("没有需要转换的图片文件，退出。")
        return 0

    stats = Stats()
    for i, src in enumerate(files, 1):
        rel = src.relative_to(src_root)
        dst = dst_root / rel.with_suffix(".webp")
        log.debug("[%d/%d] %s", i, len(files), src)

        status, in_size, out_size = convert_image(
            src, dst,
            quality=quality, method=method, lossless=lossless,
            alpha_quality=args.alpha_quality,
            overwrite=args.overwrite,
            keep_original=args.keep_original,
            log=log,
        )
        if status == "converted":
            stats.converted += 1
            stats.total_bytes_in += in_size
            stats.total_bytes_out += out_size
        elif status == "skipped":
            stats.skipped += 1
        else:
            stats.failed += 1
            stats.failures.append(str(src))

    # 汇总
    log.info("-" * 60)
    log.info("转换完成: 成功 %d，跳过 %d，失败 %d（共 %d 个文件）",
             stats.converted, stats.skipped, stats.failed, len(files))
    if stats.converted:
        saved = max(stats.total_bytes_in - stats.total_bytes_out, 0)
        pct = ((1 - stats.total_bytes_out / stats.total_bytes_in) * 100
               if stats.total_bytes_in else 0)
        log.info("体积: %.2f MB -> %.2f MB（节省 %.2f MB / %.1f%%）",
                 stats.total_bytes_in / 1048576, stats.total_bytes_out / 1048576,
                 saved / 1048576, max(pct, 0))
    if stats.failed:
        log.warning("以下 %d 个文件转换失败:", stats.failed)
        for f in stats.failures:
            log.warning("  - %s", f)
    log.info("输出目录: %s", dst_root)
    return 1 if stats.failed else 0


if __name__ == "__main__":
    sys.exit(main())
