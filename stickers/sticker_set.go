package stickers

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/libost/sticker_go/config"
	C "github.com/libost/sticker_go/constants"
	"github.com/libost/sticker_go/database"
	I "github.com/libost/sticker_go/i18n"
	"github.com/libost/sticker_go/log"
	"github.com/libost/sticker_go/utils"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
)

type StickerPackLimitError struct {
	PackLength int
	Limit      int
}

func (e *StickerPackLimitError) Error() string {
	return fmt.Sprintf("sticker pack contains %d stickers, which exceeds the per-pack limit of %d", e.PackLength, e.Limit)
}

// GetStickerPack 下载贴纸包中所有贴纸并按 50MB 限制打包后返回本地 zip 路径列表。
// 已缓存的贴纸会直接复用，无需重新下载。
func GetStickerPack(b *gotgbot.Bot, stickerSetName string, uid int64, messageId int64, ctx *ext.Context) ([]string, error) {
	stickerSet, err := b.GetStickerSet(stickerSetName, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get sticker set: %v", err)
	}
	cf, err := config.Init()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %v", err)
	}
	packLength := len(stickerSet.Stickers)
	packLimit := cf.General.LimitPerPack
	if cf.Donation.BonusEnabled {
		userGroup, err := database.Init("user_group", uid, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to get user group for donation bonus: %v", err)
		}
		if userGroup["user_group"].(string) == "sponsor" {
			packLimit = int(float64(packLimit) * C.DonationBonusMultiplier)
		}
	}
	if packLength > packLimit {
		return nil, &StickerPackLimitError{PackLength: packLength, Limit: packLimit}
	}

	tempDir := fmt.Sprintf("%s/%d", C.CacheDir, uid)
	os.MkdirAll(tempDir, 0755)
	var filePaths []string
	tgsContained := false
	tgsFileIDs := make([]string, 0)
	progressNow := 0
	stopAction := make(chan struct{})
	var stopActionOnce sync.Once
	stopActionLoop := func() {
		stopActionOnce.Do(func() {
			close(stopAction)
		})
	}
	go func() {
		_, _ = b.SendChatAction(ctx.EffectiveUser.Id, "typing", nil)
		ticker := time.NewTicker(4 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				_, _ = b.SendChatAction(ctx.EffectiveUser.Id, "typing", nil)
			case <-stopAction:
				return
			}
		}
	}()
	defer stopActionLoop()
	for _, sticker := range stickerSet.Stickers {
		var fileExt, fileExtConverted string
		switch {
		case sticker.IsAnimated:
			if cf.General.TgsSupport {
				fileExt, fileExtConverted = ".tgs", ".gif"
			} else {
				fileExt, fileExtConverted = ".tgs", ".tgs"
			}
		case sticker.IsVideo:
			fileExt, fileExtConverted = ".webm", ".gif"
		default:
			fileExt, fileExtConverted = ".webp", ".png"
		}
		progressNow++
		if progressNow == 1 || progressNow%5 == 0 || progressNow == packLength {
			langCode := I.LangCodePrefer(ctx.EffectiveUser.Id, ctx.EffectiveUser.LanguageCode)
			_, _, err = b.EditMessageText(fmt.Sprintf(I.GetLocalisedString("stickers.pack_progress", langCode), stickerSetName, progressNow, packLength), &gotgbot.EditMessageTextOpts{
				ChatId:    uid,
				MessageId: messageId,
			})
		}

		// 优先使用缓存，把缓存的文件复制到临时目录进行处理，避免直接在缓存目录进行转换操作导致的并发问题
		cachedPath := C.CacheDir + sticker.FileId + fileExtConverted
		if _, statErr := os.Stat(cachedPath); statErr == nil {
			sourceInfo, err := os.Stat(cachedPath)
			if err != nil {
				return nil, fmt.Errorf("failed to stat cached file %s: %v", cachedPath, err)
			}
			input, err := os.Open(cachedPath)
			if err != nil {
				return nil, fmt.Errorf("failed to open cached file %s: %v", cachedPath, err)
			}
			defer input.Close()
			outputPath := tempDir + "/" + sticker.FileId + fileExtConverted
			output, err := os.OpenFile(outputPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, sourceInfo.Mode())
			if err != nil {
				return nil, fmt.Errorf("failed to create temp file %s: %v", outputPath, err)
			}
			io.Copy(output, input)
			filePaths = append(filePaths, outputPath)
			continue
		}

		// 获取文件下载链接
		file, err := b.GetFile(sticker.FileId, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to get file info for %s: %v", sticker.FileId, err)
		}
		downloadURL := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", b.Token, file.FilePath)
		resp, err := http.Get(downloadURL)
		if err != nil {
			return nil, fmt.Errorf("failed to download sticker %s: %v", sticker.FileId, err)
		}

		// 保存原始文件
		rawPath := C.CacheDir + sticker.FileId + fileExt
		out, err := os.Create(rawPath)
		if err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("failed to create file %s: %v", rawPath, err)
		}
		_, err = io.Copy(out, resp.Body)
		out.Close()
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to save sticker %s: %v", sticker.FileId, err)
		}

		// 格式转换
		var convertedPath string
		switch fileExt {
		case ".webp":
			convertedPath, err = utils.DecodeWebPToPNG(rawPath)
			if err != nil {
				return nil, err
			}
		case ".webm":
			convertedPath, err = utils.DecodeWebMToGIF(rawPath)
			if err != nil {
				return nil, err
			}
		default:
			tgsContained = true
			if cf.General.TgsSupport {
				convertedPath = tempDir + "/" + sticker.FileId + ".tgs" + ".gif"
				tgsFileIDs = append(tgsFileIDs, sticker.FileId)
			} else {
				convertedPath = rawPath
			}
		}

		// 仅在转换输出不是原始文件时删除原始文件。
		if convertedPath != rawPath && fileExt != ".tgs" {
			os.Remove(rawPath)
		}

		filePaths = append(filePaths, convertedPath)
	}
	if tgsContained && cf.General.TgsSupport {
		langCode := I.LangCodePrefer(ctx.EffectiveUser.Id, ctx.EffectiveUser.LanguageCode)
		_, _, _ = b.EditMessageText(I.GetLocalisedString("stickers.tgs_converting", langCode), &gotgbot.EditMessageTextOpts{
			ChatId:    uid,
			MessageId: messageId,
		})
		err = utils.DecodeTgsToGIF(tempDir)
		if err != nil {
			if errors.Is(err, utils.ErrTgsConversionUnsupported) {
				for _, fileID := range tgsFileIDs {
					gifPath := tempDir + "/" + fileID + ".tgs" + ".gif"
					tgsPath := tempDir + "/" + fileID + ".tgs"
					for i := range filePaths {
						if filePaths[i] == gifPath {
							filePaths[i] = tgsPath
						}
					}
				}
				log.Log("TGS->GIF conversion is unsupported in this environment, keeping original TGS files for any animated stickers", C.LogLevelWarn)
			} else {
				return nil, fmt.Errorf("failed to convert TGS to GIF: %v", err)
			}
		} else {
			for _, fileID := range tgsFileIDs {
				gifPath := tempDir + "/" + fileID + ".tgs" + ".gif"
				tgsPath := tempDir + "/" + fileID + ".tgs"
				if _, statErr := os.Stat(gifPath); os.IsNotExist(statErr) {
					for i := range filePaths {
						if filePaths[i] == gifPath {
							filePaths[i] = tgsPath
						}
					}
					log.Log(fmt.Sprintf("Converted GIF not found for %s, fallback to original TGS", fileID), C.LogLevelWarn)
				} else if statErr != nil {
					return nil, fmt.Errorf("failed to stat converted gif %s: %v", gifPath, statErr)
				}
			}
			tgsFiles, globErr := filepath.Glob(filepath.Join(tempDir, "*.tgs"))
			if globErr == nil {
				for _, tgsFile := range tgsFiles {
					_ = os.Remove(tgsFile)
				}
			}
			pattern := filepath.Join(tempDir, "*.json")
			jsonFiles, globErr := filepath.Glob(pattern)
			if globErr == nil {
				for _, jsonFile := range jsonFiles {
					_ = os.Remove(jsonFile)
				}
			}
			log.Log("Successfully converted TGS stickers to GIF format", C.LogLevelInfo)
		}

	}
	zipPaths, err := buildStickerPackZips(stickerSetName, filePaths, tempDir)
	if err != nil {
		return nil, err
	}
	if !cf.Cache.Enabled {
		for _, path := range filePaths {
			os.Remove(path) // 如果缓存未启用，处理完成后删除文件
			log.Log(fmt.Sprintf("Cache disabled, removed file: %s", path), C.LogLevelInfo)
		}
	} else {
		// 贴纸包处理完成后移动下载的文件到缓存目录，供后续同一贴纸包的下载使用
		for _, path := range filePaths {
			cachedPath := C.CacheDir + filepath.Base(path)
			_, err := os.Stat(cachedPath)
			if err == nil {
				log.Log(fmt.Sprintf("File already exists in cache, skipping move: %s", cachedPath), C.LogLevelInfo)
				continue
			}
			if err := os.Rename(path, cachedPath); err != nil {
				log.Log(fmt.Sprintf("Failed to move file to cache: %s", err), C.LogLevelError)
			}
		}
	}
	usage := max(math.Ceil(float64(len(stickerSet.Stickers))/2)-1, 1) // 按照每 2 个贴纸计 1 次使用，向上取整，最后减去 1 次（因为第一次使用不计数）,最少计 1 次
	database.Init("usageRecord", uid, map[string]any{"usage": usage})
	stopActionLoop() // 停止发送 typing action
	return zipPaths, nil
}

func addFileToZip(archive *zip.Writer, filename string) error {
	// 打开源文件
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return err
	}

	// 在 zip 中创建一条记录（仅使用文件名，不含路径）
	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	header.Name = filepath.Base(filename)
	header.Method = zip.Store
	writer, err := archive.CreateHeader(header)
	if err != nil {
		return err
	}

	// 将源文件内容拷贝到 zip 记录中
	_, err = io.Copy(writer, file)
	return err
}

func buildStickerPackZips(stickerSetName string, filePaths []string, tempDir string) ([]string, error) {
	if len(filePaths) == 0 {
		return nil, fmt.Errorf("sticker pack %s is empty", stickerSetName)
	}

	var (
		zipPaths            []string
		currentZip          *os.File
		currentZipWriter    *zip.Writer
		currentZipPath      string
		currentZipSize      int64
		currentZipFileCount int
	)

	closeCurrentZip := func() error {
		if currentZipWriter == nil || currentZip == nil {
			return nil
		}
		if err := currentZipWriter.Close(); err != nil {
			currentZipWriter = nil
			closeErr := currentZip.Close()
			currentZip = nil
			if closeErr != nil {
				return fmt.Errorf("failed to close zip writer: %v; failed to close zip file: %v", err, closeErr)
			}
			return fmt.Errorf("failed to close zip writer: %v", err)
		}
		if err := currentZip.Close(); err != nil {
			currentZip = nil
			currentZipWriter = nil
			return fmt.Errorf("failed to close zip file: %v", err)
		}
		currentZip = nil
		currentZipWriter = nil
		currentZipPath = ""
		currentZipSize = 0
		currentZipFileCount = 0
		return nil
	}

	cleanupZipPaths := func(paths []string) {
		for _, path := range paths {
			_ = os.Remove(path)
		}
	}

	createNextZip := func(part int) error {
		suffix := ".zip"
		if len(filePaths) > 1 {
			suffix = fmt.Sprintf(".part%d.zip", part)
		}
		currentZipPath = filepath.Join(tempDir, sanitizeZipBaseName(stickerSetName)+suffix)
		file, err := os.Create(currentZipPath)
		if err != nil {
			return fmt.Errorf("failed to create zip file: %v", err)
		}
		currentZip = file
		currentZipWriter = zip.NewWriter(file)
		currentZipSize = C.ZipArchiveFooterSize
		currentZipFileCount = 0
		zipPaths = append(zipPaths, currentZipPath)
		return nil
	}

	part := 1
	if err := createNextZip(part); err != nil {
		return nil, err
	}

	for _, path := range filePaths {
		entrySize, err := estimateZipEntrySize(path)
		if err != nil {
			_ = closeCurrentZip()
			cleanupZipPaths(zipPaths)
			return nil, err
		}
		if entrySize > C.MaxZipPartSizeBytes {
			_ = closeCurrentZip()
			cleanupZipPaths(zipPaths)
			return nil, fmt.Errorf("sticker file %s exceeds zip part size limit", filepath.Base(path))
		}

		if currentZipFileCount > 0 && currentZipSize+entrySize > C.MaxZipPartSizeBytes {
			if err := closeCurrentZip(); err != nil {
				cleanupZipPaths(zipPaths)
				return nil, err
			}
			part++
			if err := createNextZip(part); err != nil {
				cleanupZipPaths(zipPaths)
				return nil, err
			}
		}

		if err := addFileToZip(currentZipWriter, path); err != nil {
			_ = closeCurrentZip()
			cleanupZipPaths(zipPaths)
			return nil, fmt.Errorf("failed to add file to zip: %v", err)
		}
		currentZipSize += entrySize
		currentZipFileCount++
	}

	if err := closeCurrentZip(); err != nil {
		cleanupZipPaths(zipPaths)
		return nil, err
	}

	if len(zipPaths) == 1 && strings.HasSuffix(zipPaths[0], ".part1.zip") {
		newPath := filepath.Join(tempDir, sanitizeZipBaseName(stickerSetName)+".zip")
		if err := os.Rename(zipPaths[0], newPath); err != nil {
			cleanupZipPaths(zipPaths)
			return nil, fmt.Errorf("failed to rename zip file: %v", err)
		}
		zipPaths[0] = newPath
	}

	return zipPaths, nil
}

func estimateZipEntrySize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, fmt.Errorf("failed to stat file %s: %v", path, err)
	}
	nameLen := int64(len(filepath.Base(path)))
	return info.Size() + C.ZipEntryHeaderSize + nameLen*2, nil
}

func sanitizeZipBaseName(name string) string {
	replacer := strings.NewReplacer(
		"/", "_",
		"\\", "_",
		":", "_",
		"*", "_",
		"?", "_",
		"\"", "_",
		"<", "_",
		">", "_",
		"|", "_",
	)
	return replacer.Replace(name)
}
