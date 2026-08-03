# -*- coding: utf-8 -*-
"""
外部插件示例：新增 png2jpg 转换器（PNG -> JPEG）。

使用方式:
    python batch_convert.py -c config.json --plugin examples/plugin_png2jpg.py

插件开发三步:
    1. from batch_convert import Converter, register
    2. 定义转换器类，覆写 name / output_name() / convert()（可选 validate_params()）
    3. 用 @register 装饰器注册（或定义 PLUGINS = [PngToJpgConverter] 列表）
之后即可在配置文件中用 "converter": "png2jpg" 引用。

注意事项:
    - 插件文件独立于主程序，可放在任意位置，通过 --plugin 指定路径
    - 依赖库（如 Pillow）在 convert() 内延迟导入，避免加载插件时即报错
    - 转换失败时抛出异常即可，调度器会自动重试并计入统计
"""
from pathlib import Path

from batch_convert import Converter, register


@register
class PngToJpgConverter(Converter):
    """PNG 转 JPEG（演示外部插件扩展）。"""

    name = "png2jpg"

    def output_name(self, src: Path, params: dict) -> str:
        return src.stem + ".jpg"

    def validate_params(self, params: dict):
        if not 0 <= params.get("quality", 90) <= 100:
            return "quality 必须在 0-100 之间"
        return None

    def convert(self, src: Path, dst: Path, params: dict) -> None:
        # 延迟导入，仅在使用该转换器时才需要 Pillow
        from PIL import Image, ImageOps

        quality = params.get("quality", 90)
        with Image.open(src) as im:
            im = ImageOps.exif_transpose(im)
            # JPEG 不支持透明通道，先拍平到 RGB
            if im.mode not in ("RGB", "L"):
                background = Image.new("RGB", im.size, (255, 255, 255))
                background.paste(im, mask=im.split()[-1] if "A" in im.mode else None)
                im = background
            dst.parent.mkdir(parents=True, exist_ok=True)
            im.save(dst, "JPEG", quality=quality)
