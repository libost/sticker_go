package stickers

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"libost/sticker_go/config"
	C "libost/sticker_go/constants"
	"libost/sticker_go/database"
	"libost/sticker_go/utils"

	"github.com/PaulSonOfLars/gotgbot/v2"
)

// GetStickerPack 下载贴纸包中所有贴纸并返回本地文件路径列表。
// 已缓存的贴纸会直接复用，无需重新下载。
func GetStickerPack(b *gotgbot.Bot, stickerSetName string, uid int64) (string, error) {
	stickerSet, err := b.GetStickerSet(stickerSetName, nil)
	if err != nil {
		return "", fmt.Errorf("failed to get sticker set: %v", err)
	}
	cf, err := config.Init()
	if err != nil {
		return "", fmt.Errorf("failed to load config: %v", err)
	}
	packLength := len(stickerSet.Stickers)
	if packLength > cf.LimitPerPack {
		tms := fmt.Sprintf("too_many_stickers_%d_%d", packLength, cf.LimitPerPack)
		return tms, fmt.Errorf("sticker pack contains %d stickers, which exceeds the per-pack limit of %d", packLength, cf.LimitPerPack)
	}

	var filePaths []string
	for _, sticker := range stickerSet.Stickers {
		var fileExt, fileExtConverted string
		switch {
		case sticker.IsAnimated:
			fileExt, fileExtConverted = ".tgs", ".tgs"
		case sticker.IsVideo:
			fileExt, fileExtConverted = ".webm", ".gif"
		default:
			fileExt, fileExtConverted = ".webp", ".png"
		}

		// 优先使用缓存
		cachedPath := C.CacheDir + sticker.FileId + fileExtConverted
		if _, statErr := os.Stat(cachedPath); statErr == nil {
			filePaths = append(filePaths, cachedPath)
			continue
		}

		// 获取文件下载链接
		file, err := b.GetFile(sticker.FileId, nil)
		if err != nil {
			return "", fmt.Errorf("failed to get file info for %s: %v", sticker.FileId, err)
		}
		downloadURL := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", b.Token, file.FilePath)
		resp, err := http.Get(downloadURL)
		if err != nil {
			return "", fmt.Errorf("failed to download sticker %s: %v", sticker.FileId, err)
		}

		// 保存原始文件
		rawPath := C.CacheDir + sticker.FileId + fileExt
		out, err := os.Create(rawPath)
		if err != nil {
			resp.Body.Close()
			return "", fmt.Errorf("failed to create file %s: %v", rawPath, err)
		}
		_, err = io.Copy(out, resp.Body)
		out.Close()
		resp.Body.Close()
		if err != nil {
			return "", fmt.Errorf("failed to save sticker %s: %v", sticker.FileId, err)
		}

		// 格式转换
		var convertedPath string
		switch fileExt {
		case ".webp":
			convertedPath, err = utils.DecodeWebPToPNG(sticker.FileId)
			if err != nil {
				return "", err
			}
		case ".webm":
			convertedPath, err = utils.DecodeWebMToGIF(sticker.FileId)
			if err != nil {
				return "", err
			}
		default:
			convertedPath = rawPath
		}

		// 转换后删除原始文件（tgs 不转换，直接保留）
		if fileExt != fileExtConverted {
			os.Remove(rawPath)
		}

		filePaths = append(filePaths, convertedPath)
	}
	// 将所有贴纸打包成一个 zip 文件，方便用户下载
	outZipPath := fmt.Sprintf("%s%s.zip", C.CacheDir, stickerSetName)
	outZip, err := os.Create(outZipPath)
	if err != nil {
		return "", fmt.Errorf("failed to create zip file: %v", err)
	}
	defer outZip.Close()
	zipWriter := zip.NewWriter(outZip)
	defer zipWriter.Close()
	for _, path := range filePaths {
		if err := addFileToZip(zipWriter, path); err != nil {
			return "", fmt.Errorf("failed to add file to zip: %v", err)
		}
	}
	database.Init("usageRecord", uid, map[string]any{"usage": len(stickerSet.Stickers)})
	return outZipPath, nil
}

func addFileToZip(archive *zip.Writer, filename string) error {
	// 打开源文件
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	// 在 zip 中创建一条记录（仅使用文件名，不含路径）
	writer, err := archive.Create(filepath.Base(filename))
	if err != nil {
		return err
	}

	// 将源文件内容拷贝到 zip 记录中
	_, err = io.Copy(writer, file)
	return err
}
