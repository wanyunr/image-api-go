#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
batch_convert - 配置化的多文件夹批量文件转换工具

通过 JSON/YAML 配置文件定义多个转换任务，每个任务独立指定：
源目录、输出目录、文件过滤规则、转换器（插件）与参数。
支持递归扫描、断点续传（跳过已存在输出）、失败自动重试、
外部插件扩展，并输出成功/跳过/失败统计日志。

用法示例:
    python batch_convert.py --config config.json
    python batch_convert.py -c config.json --log-file run.log --verbose
    python batch_convert.py -c config.json --plugin ./my_plugin.py

内置转换器:
    image_webp  图片转 WebP（PNG/JPG/JPEG/BMP/TIFF/GIF -> WebP）
    copy        原样复制文件（基础示范）

外部插件约定: 在任意 .py 文件中定义转换器类并注册，例如
    from batch_convert import Converter, register

    @register
    class MyConverter(Converter):
        name = "my_convert"
        def output_name(self, src, params): return src.stem + ".out"
        def convert(self, src, dst, params): ...

配置格式（详见 README.md）:
    {
      "tasks": [
        {
          "name": "照片转webp",
          "converter": "image_webp",
          "source": "C:/photos",
          "target": "C:/photos_webp",
          "recursive": true,
          "include": ["*.png", "*.jpg", "*.jpeg", "*.bmp", "*.tiff", "*.gif"],
          "exclude": ["*.bak"],
          "params": {"quality": 82, "method": 4},
          "skip_existing": true,
          "retries": 2
        }
      ]
    }
"""

from __future__ import annotations

import argparse
import importlib.util
import json
import logging
import os
import shutil
import sys
import time
from dataclasses import dataclass, field
from fnmatch import fnmatch
from pathlib import Path
from typing import Any, Optional

# ---------------------------------------------------------------------------
# 插件注册表
# ---------------------------------------------------------------------------

class Converter:
    """转换器基类（接口）。

    子类需实现 convert()，可覆写 output_name() 与 validate_params()。
    """

    #: 转换器名称，配置文件中 converter 字段使用该名称
    name = "base"

    def output_name(self, src: Path, params: dict) -> str:
        """决定输出文件名（默认与原文件同名）。"""
        return src.name

    def validate_params(self, params: dict) -> Optional[str]:
        """校验参数，返回错误信息；None 表示通过。"""
        return None

    def convert(self, src: Path, dst: Path, params: dict) -> None:
        """执行转换：读取 src，写出 dst。失败时抛出异常（由调度器重试）。"""
        raise NotImplementedError


REGISTRY: dict[str, type[Converter]] = {}


def register(cls: type[Converter]) -> type[Converter]:
    """类装饰器：把转换器注册到全局注册表。"""
    if cls.name in REGISTRY:
        raise ValueError(f"转换器名称重复: {cls.name}")
    REGISTRY[cls.name] = cls
    return cls


# ---------------------------------------------------------------------------
# 内置转换器
# ---------------------------------------------------------------------------

# WebP 质量分级预设: 名称 -> (quality, method, lossless)
TIERS = {
    "draft":    (50, 6, False),
    "balanced": (75, 4, False),
    "photo":    (82, 4, False),
    "high":     (90, 5, False),
    "lossless": (100, 4, True),
}


@register
class ImageWebpConverter(Converter):
    """图片转 WebP：PNG/JPG/JPEG/BMP/TIFF/GIF -> WebP。

    参数 (params):
        tier          质量预设（draft/balanced/photo/high/lossless），优先于下述细粒度参数
        quality       0-100，默认 75
        method        0-6 压缩努力度，默认 4
        lossless      是否无损，默认 false
        alpha_quality 透明区域画质 0-100，默认跟随 quality
    """

    name = "image_webp"

    def output_name(self, src: Path, params: dict) -> str:
        return src.stem + ".webp"

    def validate_params(self, params: dict) -> Optional[str]:
        if "tier" in params and params["tier"] not in TIERS:
            return f"未知质量预设 tier={params['tier']}，可选: {', '.join(sorted(TIERS))}"
        if not 0 <= params.get("quality", 75) <= 100:
            return "quality 必须在 0-100 之间"
        if not 0 <= params.get("method", 4) <= 6:
            return "method 必须在 0-6 之间"
        if not 0 <= params.get("alpha_quality", 100) <= 100:
            return "alpha_quality 必须在 0-100 之间"
        return None

    def convert(self, src: Path, dst: Path, params: dict) -> None:
        # 延迟导入：仅在使用该转换器时依赖 Pillow
        from PIL import Image, ImageOps

        # 已是 WebP 的文件直接拷贝，不做二次压缩
        if src.suffix.lower() == ".webp":
            with Image.open(src) as check:   # 校验完整性，损坏文件在此抛异常
                check.verify()
            dst.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(src, dst)
            return

        if "tier" in params:
            quality, method, lossless = TIERS[params["tier"]]
        else:
            quality = params.get("quality", 75)
            method = params.get("method", 4)
            lossless = params.get("lossless", False)
        alpha_quality = params.get("alpha_quality")

        # 校验图片完整性，损坏文件在此抛出异常
        with Image.open(src) as check:
            check.verify()

        with Image.open(src) as im:
            im = ImageOps.exif_transpose(im)   # 修正 EXIF 方向
            im.seek(0)                          # 多帧格式（GIF）取第一帧
            if im.mode in ("RGBA", "LA") or (im.mode == "P" and "transparency" in im.info):
                im = im.convert("RGBA")
            else:
                im = im.convert("RGB")

            save_kwargs = {"quality": quality, "method": method, "lossless": lossless}
            if alpha_quality is not None:
                save_kwargs["alpha_quality"] = alpha_quality
            dst.parent.mkdir(parents=True, exist_ok=True)
            im.save(dst, "WEBP", **save_kwargs)


@register
class CopyConverter(Converter):
    """原样复制文件（最基础的转换器，用于演示与测试）。"""

    name = "copy"

    def convert(self, src: Path, dst: Path, params: dict) -> None:
        dst.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(src, dst)


# ---------------------------------------------------------------------------
# 外部插件加载
# ---------------------------------------------------------------------------

def load_external_plugins(plugin_paths: list[str]) -> list[str]:
    """动态加载外部插件文件/目录，返回加载的模块列表。

    插件文件内通过 `from batch_convert import Converter, register` 注册转换器，
    或定义 `PLUGINS = [...]`（转换器类列表）由本函数代为注册。
    """
    loaded: list[str] = []
    files: list[Path] = []
    for p in plugin_paths:
        path = Path(p).expanduser().resolve()
        if path.is_dir():
            files.extend(sorted(f for f in path.glob("*.py") if not f.name.startswith("_")))
        elif path.is_file() and path.suffix == ".py":
            files.append(path)
        else:
            raise FileNotFoundError(f"插件路径不存在: {p}")

    for f in files:
        module_name = f"bc_plugin_{f.stem}"
        spec = importlib.util.spec_from_file_location(module_name, f)
        if spec is None or spec.loader is None:
            raise ImportError(f"无法加载插件模块: {f}")
        module = importlib.util.module_from_spec(spec)
        sys.modules[module_name] = module
        spec.loader.exec_module(module)
        # 兼容两种注册方式：模块内自注册，或提供 PLUGINS 列表
        for cls in getattr(module, "PLUGINS", []):
            register(cls)
        loaded.append(str(f))
    return loaded


# ---------------------------------------------------------------------------
# 任务模型与校验
# ---------------------------------------------------------------------------

@dataclass
class TaskConfig:
    """单个转换任务的配置。"""

    name: str
    converter: str
    source: Path
    target: Path
    recursive: bool = True
    include: list = field(default_factory=list)
    exclude: list = field(default_factory=list)
    params: dict = field(default_factory=dict)
    skip_existing: bool = True
    retries: int = 2
    flat: bool = False

    #: 允许直接写在任务顶层（平铺）的转换器参数键，无需嵌套 params
    PARAM_KEYS = ("quality", "method", "tier", "lossless", "alpha_quality")

    #: 未显式指定 include 时的默认过滤：处理所有常见图片格式
    #: （webp 直接拷贝；想处理所有文件请显式写 include: ["*"]）
    DEFAULT_INCLUDE = ["*.png", "*.jpg", "*.jpeg", "*.bmp",
                       "*.tif", "*.tiff", "*.gif", "*.webp"]

    @classmethod
    def from_dict(cls, cfg: dict, index: int = 0) -> "TaskConfig":
        name = str(cfg.get("name") or f"task-{index + 1}")
        source = Path(cfg["source"]).expanduser().resolve()
        target = (Path(cfg["target"]).expanduser().resolve()
                  if cfg.get("target")
                  else source.with_name(source.name + "_converted"))
        # 参数来源：嵌套 params 优先，其次顶层平铺（quality/tier 等）
        params = dict(cfg.get("params") or {})
        for key in cls.PARAM_KEYS:
            if key in cfg and key not in params:
                params[key] = cfg[key]
        return cls(
            name=name,
            converter=cfg.get("converter", "image_webp"),  # 默认图片转 WebP
            source=source,
            target=target,
            recursive=bool(cfg.get("recursive", True)),
            include=list(cfg.get("include") or cls.DEFAULT_INCLUDE),
            exclude=list(cfg.get("exclude") or []),
            params=params,
            skip_existing=bool(cfg.get("skip_existing", True)),
            retries=int(cfg.get("retries", 2)),
            flat=bool(cfg.get("flat", False)),
        )


def validate_task(task: TaskConfig, log: logging.Logger) -> bool:
    """校验任务配置，返回是否合法。"""
    ok = True
    if task.converter not in REGISTRY:
        log.error("[%s] 未知转换器: %s（可用: %s）",
                  task.name, task.converter, ", ".join(sorted(REGISTRY)))
        ok = False
    else:
        err = REGISTRY[task.converter]().validate_params(task.params)
        if err:
            log.error("[%s] 转换器 %s 参数错误: %s", task.name, task.converter, err)
            ok = False
    if not task.source.is_dir():
        log.error("[%s] 源目录不存在或不是目录: %s", task.name, task.source)
        ok = False
    if task.retries < 0:
        log.error("[%s] retries 不能为负数", task.name)
        ok = False
    return ok


def load_config(path: Path) -> list[dict]:
    """读取配置文件，返回任务字典列表。

    支持三种结构:
        极简单任务:  {source, target, ...}（source 为字符串）
        极简多组:    {source: [a, b], target: [a1, b1], ...}（一一对应，公共参数共享）
        多任务:      {tasks: [{source, target, ...}, ...]}
    .json 用内置 json；.yaml/.yml 需要 pyyaml。
    """
    if path.suffix.lower() in (".yaml", ".yml"):
        try:
            import yaml
        except ImportError:
            raise SystemExit(
                f"配置文件 {path} 是 YAML 格式，请先安装: pip install pyyaml")
        with path.open(encoding="utf-8") as f:
            data = yaml.safe_load(f)
    else:
        with path.open(encoding="utf-8") as f:
            data = json.load(f)

    if not isinstance(data, dict):
        raise SystemExit("配置文件顶层必须是对象（{source, target} 或 {tasks: [...]}）")
    if "tasks" in data:
        if not isinstance(data["tasks"], list) or not data["tasks"]:
            raise SystemExit("配置文件必须包含非空的 tasks 数组")
        return data["tasks"]
    if "source" in data:
        return _expand_single(data)
    raise SystemExit("配置缺少 source 字段（极简格式）或 tasks 数组（多任务格式）")


def _expand_single(data: dict) -> list[dict]:
    """展开极简单任务/多组配置为任务字典列表。

    source 为字符串 -> 单个任务；
    source 为列表   -> 多组任务，target 列表与其一一对应（省略 target 时自动命名）。
    """
    src = data["source"]
    if isinstance(src, str):
        return [data]

    if not isinstance(src, list) or not src:
        raise SystemExit("source 必须是路径字符串或非空路径列表")

    tgt = data.get("target")
    if isinstance(tgt, str):
        raise SystemExit("source 为多个时，target 也必须是等长列表（或省略 target 自动命名）")
    if tgt is not None and len(tgt) != len(src):
        raise SystemExit(f"source 与 target 数量不一致: {len(src)} 个源 vs {len(tgt)} 个目标")

    tasks = []
    for i, s in enumerate(src):
        item = dict(data)          # 公共字段（converter/tier 等）共享
        item["source"] = s
        if tgt is not None:
            item["target"] = tgt[i]
        else:
            item.pop("target", None)   # 由 TaskConfig 按 <源目录名>_converted 自动派生
        tasks.append(item)
    return tasks


# ---------------------------------------------------------------------------
# 扫描与执行
# ---------------------------------------------------------------------------

def scan_files(task: TaskConfig) -> list[Path]:
    """扫描源目录下符合过滤规则的文件（可选排除输出目录子树）。"""
    pattern = "**/*" if task.recursive else "*"
    files: list[Path] = []
    for p in task.source.glob(pattern):
        if not p.is_file():
            continue
        rel = p.relative_to(task.source).as_posix()
        # 输出目录位于源目录内部时排除，避免重复处理
        if task.target == p or task.target in p.parents:
            continue
        if task.include and not any(fnmatch(rel, pat) for pat in task.include):
            continue
        if any(fnmatch(rel, pat) for pat in task.exclude):
            continue
        files.append(p)
    return files


def output_path(src: Path, task: TaskConfig, converter: Converter) -> Path:
    """计算输出文件路径：默认保留相对目录结构；flat=True 时平铺到目标根目录。"""
    if task.flat:
        return task.target / converter.output_name(src, task.params)
    rel_dir = src.relative_to(task.source).parent
    return task.target / rel_dir / converter.output_name(src, task.params)


def process_file(converter: Converter, src: Path, dst: Path,
                 params: dict, retries: int, log: logging.Logger) -> str:
    """转换单个文件（串行模式），失败自动重试（每次尝试前清理半成品）。"""
    for attempt in range(retries + 1):
        try:
            converter.convert(src, dst, params)
            return "converted"
        except Exception as exc:  # noqa: BLE001 转换器异常统一按失败处理
            dst.unlink(missing_ok=True)  # 清理可能的半成品
            if attempt < retries:
                delay = 0.5 * (attempt + 1)
                log.warning("%s 第 %d 次尝试失败（%s），%.1fs 后重试: %s",
                            src.name, attempt + 1, type(exc).__name__, delay, exc)
                time.sleep(delay)
            else:
                log.error("%s 失败（已重试 %d 次）: %s: %s",
                          src.name, retries, type(exc).__name__, exc)
    return "failed"


def _init_worker(plugin_paths: tuple) -> None:
    """子进程启动时执行：使插件转换器在子进程中可用。

    Windows spawn 模式下子进程以 __mp_main__ 导入主脚本，__main__ 块不会执行，
    这里显式把子进程主模块注册为 batch_convert，插件里
    `from batch_convert import ...` 才能引用到与 worker 相同的 REGISTRY。
    """
    if sys.modules.get("batch_convert") is not sys.modules[__name__]:
        sys.modules["batch_convert"] = sys.modules[__name__]
    for path in plugin_paths:
        try:
            load_external_plugins([path])
        except Exception as exc:  # noqa: BLE001
            print(f"[worker] 插件加载失败: {exc}", file=sys.stderr)


def _convert_worker(args: tuple) -> tuple:
    """子进程 worker：转换单个文件（无日志，返回结构化结果）。

    返回 ("converted", None) 或 ("failed", 错误描述)。
    """
    converter_name, src, dst, params, retries = args
    converter = REGISTRY[converter_name]()
    for attempt in range(retries + 1):
        try:
            converter.convert(Path(src), Path(dst), params)
            return ("converted", None)
        except Exception as exc:  # noqa: BLE001
            try:
                Path(dst).unlink(missing_ok=True)  # 清理半成品
            except OSError:
                pass
            if attempt < retries:
                time.sleep(0.5 * (attempt + 1))
            else:
                return ("failed", f"{type(exc).__name__}: {exc}")
    return ("failed", "unknown")


@dataclass
class Stats:
    """任务统计。"""

    converted: int = 0
    skipped: int = 0
    failed: int = 0
    failures: list = field(default_factory=list)


def run_task(task: TaskConfig, log: logging.Logger,
             workers: int = 1, plugin_paths: tuple = ()) -> Stats:
    """执行一个转换任务，返回统计。workers > 1 时用多进程并行转换文件。"""
    stats = Stats()
    converter = REGISTRY[task.converter]()

    try:
        task.target.mkdir(parents=True, exist_ok=True)
    except OSError as exc:
        log.error("[%s] 无法创建输出目录: %s (%s)", task.name, task.target, exc)
        stats.failed += 1
        stats.failures.append(f"<无法创建输出目录> {task.target}")
        return stats

    files = scan_files(task)
    log.info("[%s] 转换器=%s | 源=%s | 目标=%s | 共 %d 个文件%s",
             task.name, task.converter, task.source, task.target, len(files),
             f" | 并行 {workers} 进程" if workers > 1 else "")
    if not files:
        log.info("[%s] 没有符合条件的文件，跳过该任务", task.name)
        return stats

    if workers > 1:
        _run_task_parallel(task, converter, files, workers, plugin_paths, stats, log)
    else:
        _run_task_serial(task, converter, files, stats, log)

    log.info("[%s] 完成: 成功 %d，跳过 %d，失败 %d（共 %d 个文件）",
             task.name, stats.converted, stats.skipped, stats.failed, len(files))
    if stats.failures:
        log.warning("[%s] 失败文件清单:", task.name)
        for f in stats.failures:
            log.warning("  - %s", f)
    return stats


def _run_task_serial(task: TaskConfig, converter: Converter, files: list,
                     stats: Stats, log: logging.Logger) -> None:
    """串行执行（workers=1）。"""
    for i, src in enumerate(files, 1):
        dst = output_path(src, task, converter)
        log.debug("[%s] [%d/%d] %s", task.name, i, len(files), src.name)

        if task.skip_existing and dst.exists():
            log.info("[%s] 跳过（目标已存在）: %s", task.name, dst)
            stats.skipped += 1
            continue

        status = process_file(converter, src, dst, task.params, task.retries, log)
        if status == "converted":
            stats.converted += 1
            log.info("[%s] 转换成功: %s -> %s", task.name, src.name, dst.name)
        else:
            stats.failed += 1
            stats.failures.append(str(src))


def _run_task_parallel(task: TaskConfig, converter: Converter, files: list,
                       workers: int, plugin_paths: tuple,
                       stats: Stats, log: logging.Logger) -> None:
    """多进程并行执行（workers > 1）：按文件分发到进程池，日志在主进程汇总。"""
    from concurrent.futures import ProcessPoolExecutor, as_completed

    # 预检查跳过项，避免不必要的进程调度
    jobs: list[tuple] = []
    for i, src in enumerate(files, 1):
        dst = output_path(src, task, converter)
        if task.skip_existing and dst.exists():
            log.info("[%s] 跳过（目标已存在）: %s", task.name, dst)
            stats.skipped += 1
            continue
        jobs.append((src, dst))

    if not jobs:
        return

    pending = {}
    with ProcessPoolExecutor(max_workers=workers,
                             initializer=_init_worker,
                             initargs=(plugin_paths,)) as executor:
        for src, dst in jobs:
            future = executor.submit(
                _convert_worker,
                (task.converter, str(src), str(dst), task.params, task.retries))
            pending[future] = (src, dst)

        # 按完成顺序输出结果（并行下日志顺序不固定属正常）
        for future in as_completed(pending):
            src, dst = pending[future]
            try:
                status, detail = future.result()
            except Exception as exc:  # noqa: BLE001 worker 意外异常
                status, detail = "failed", f"{type(exc).__name__}: {exc}"
            if status == "converted":
                stats.converted += 1
                log.info("[%s] 转换成功: %s -> %s", task.name, src.name, dst.name)
            else:
                stats.failed += 1
                stats.failures.append(str(src))
                log.error("[%s] %s 失败: %s", task.name, src.name, detail)


# ---------------------------------------------------------------------------
# CLI
# ---------------------------------------------------------------------------

def setup_logging(verbose: bool, log_file: Optional[str]) -> logging.Logger:
    logger = logging.getLogger("batch_convert")
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


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="batch_convert",
        description="配置化的多文件夹批量文件转换工具（JSON/YAML 配置驱动）。",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog=(
            "示例:\n"
            "  python batch_convert.py -c config.yaml\n"
            "  python batch_convert.py -c config.yaml --workers 4\n"
            "  python batch_convert.py -c config.yaml --plugin ./plugins\n"
            "  python batch_convert.py -c config.yaml --log-file run.log --verbose\n"
        ),
    )
    parser.add_argument("-c", "--config", metavar="FILE",
                        help="配置文件路径（.json 或 .yaml；与 --list-converters 二选一）")
    parser.add_argument("--plugin", action="append", default=[], metavar="PATH",
                        help="外部插件文件或目录（可多次指定），加载其中注册的转换器")
    parser.add_argument("--workers", type=int, default=None, metavar="N",
                        help="并行转换的进程数（默认: CPU 核数；1 为串行）")
    parser.add_argument("--log-file", metavar="FILE",
                        help="除终端外，同时将日志写入指定文件")
    parser.add_argument("--verbose", action="store_true",
                        help="输出 DEBUG 级别详细日志")
    parser.add_argument("--list-converters", action="store_true",
                        help="列出所有可用转换器后退出")
    return parser


def main(argv: Optional[list[str]] = None) -> int:
    args = build_parser().parse_args(argv)

    if sys.platform == "win32":
        for stream in (sys.stdout, sys.stderr):
            try:
                stream.reconfigure(encoding="utf-8", errors="replace")
            except (AttributeError, ValueError):
                pass

    log = setup_logging(args.verbose, args.log_file)

    # 加载外部插件（先于 --list-converters 与任务执行）
    for path in args.plugin:
        try:
            loaded = load_external_plugins([path])
            log.info("已加载插件: %s", ", ".join(loaded))
        except (FileNotFoundError, ImportError, ValueError) as exc:
            log.error("插件加载失败: %s", exc)
            return 2

    if args.list_converters:
        for name in sorted(REGISTRY):
            doc = REGISTRY[name].__doc__ or ""
            print(f"{name:12s} {doc.splitlines()[0] if doc else ''}")
        return 0

    if not args.config:
        build_parser().error("必须提供 -c/--config（或使用 --list-converters 查看可用转换器）")

    config_path = Path(args.config).expanduser().resolve()
    if not config_path.is_file():
        log.error("配置文件不存在: %s", config_path)
        return 2

    # 读取并解析配置（极简单任务 / tasks 多任务数组均可）
    try:
        tasks_raw = load_config(config_path)
    except (json.JSONDecodeError, SystemExit) as exc:
        log.error("配置解析失败: %s", exc)
        return 2

    tasks = [TaskConfig.from_dict(c, i) for i, c in enumerate(tasks_raw)]
    valid_tasks = [t for t in tasks if validate_task(t, log)]
    if not valid_tasks:
        log.error("没有合法的任务可执行，退出。")
        return 2

    # 并行进程数：--workers 缺省时取 CPU 核数（图片转换是 CPU 密集型，多进程才有效）
    workers = args.workers or os.cpu_count() or 1
    if workers < 1:
        log.error("--workers 必须 >= 1")
        return 2
    if workers > 1:
        log.info("并行模式: %d 个进程", workers)

    # 逐个执行任务（任务内文件并行，任务间顺序执行）
    total = Stats()
    for task in valid_tasks:
        log.info("-" * 60)
        s = run_task(task, log, workers=workers, plugin_paths=tuple(args.plugin))
        total.converted += s.converted
        total.skipped += s.skipped
        total.failed += s.failed
        total.failures.extend(s.failures)

    # 全局汇总
    log.info("=" * 60)
    log.info("全部任务完成: 成功 %d，跳过 %d，失败 %d",
             total.converted, total.skipped, total.failed)
    if total.failures:
        log.warning("失败文件 %d 个:", len(total.failures))
        for f in total.failures:
            log.warning("  - %s", f)
    return 1 if total.failed else 0


if __name__ == "__main__":
    # 关键：本程序以 __main__ 运行时，外部插件里的 `from batch_convert import register`
    # 会触发模块再次加载，导致插件注册到"副本"模块的 REGISTRY。
    # 这里把当前运行实例注册到 sys.modules，使插件引用到同一模块、同一注册表。
    sys.modules["batch_convert"] = sys.modules[__name__]
    sys.exit(main())
