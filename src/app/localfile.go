package app

import (
	"errors"
	"fmt"
	"os"
)

// errLocalFileChanged 表示扫描后文件已被替换（例如换成了符号链接）。
var errLocalFileChanged = errors.New("local image is no longer a regular file")

// openLocalFile 打开已扫描的本地图片，并确认路径最终仍指向刚打开的这个普通文件。
//
// 扫描阶段已跳过符号链接；这里再校验一次，防止扫描后文件被替换成指向目录外的链接：
//   - 先打开拿到 fd，后续内容只从 fd 读取；
//   - 再用 Lstat 查看路径本身（不跟随链接），与 fd 的 Stat 结果比较是否为同一文件。
//
// 若路径已是符号链接，或在校验前被换成另一个文件，SameFile 会失败并拒绝。
// 因为内容只从已校验的 fd 读取，校验之后再发生替换也不会读到别的文件。
func openLocalFile(path string) (*os.File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	opened, statErr := f.Stat()
	pathInfo, lstatErr := os.Lstat(path)
	if statErr != nil || lstatErr != nil {
		f.Close()
		if statErr != nil {
			return nil, statErr
		}
		return nil, lstatErr
	}
	if pathInfo.Mode()&os.ModeSymlink != 0 ||
		!opened.Mode().IsRegular() ||
		!os.SameFile(opened, pathInfo) {
		f.Close()
		return nil, fmt.Errorf("%w: %s", errLocalFileChanged, path)
	}
	return f, nil
}
