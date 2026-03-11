package utils

import (
	"os"
	"path/filepath"
)

// GetDirSize 计算文件夹大小（单位：字节）
func GetDirSize(path string) (int64, error) {
	var size int64
	// Walk 会递归遍历目录下的所有文件和子目录
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		// 如果不是目录，则累加文件大小
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size, err
}
