package utils

import (
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	C "libost/sticker_go/constants"

	"golang.org/x/image/webp"
)

var ErrTgsConversionUnsupported = errors.New("tgs conversion unsupported by current ffmpeg build")

func DecodeWebPToPNG(fileId string) (filePath string, err error) {
	f, err := os.Open(C.CacheDir + fileId + ".webp")
	if err != nil {
		return "", err
	}
	defer f.Close()
	img, err := webp.Decode(f)
	if err != nil {
		return "", err
	}
	outPutFile, err := os.Create(C.CacheDir + fileId + ".png")
	if err != nil {
		return "", err
	}
	defer outPutFile.Close()
	err = png.Encode(outPutFile, img)
	if err != nil {
		return "", err
	}
	return C.CacheDir + fileId + ".png", nil
}

func DecodeWebMToGIF(fileId string) (filePath string, err error) {
	// 这里需要使用第三方库来处理 WebM 视频并转换为 GIF
	// 例如，可以使用 ffmpeg 命令行工具来完成这个任务
	// 你需要确保系统上安装了 ffmpeg，并且在 PATH 中可用
	cmd := exec.Command("ffmpeg", "-i", C.CacheDir+fileId+".webm", C.CacheDir+fileId+".gif")
	err = cmd.Run()
	if err != nil {
		return "", err
	}
	return C.CacheDir + fileId + ".gif", nil
}

func DecodeTgsToGIF(filePath string) (string, error) {
	outputFilePath := strings.TrimSuffix(filePath, filepath.Ext(filePath)) + ".gif"

	// Try direct conversion first. Some ffmpeg builds can decode .tgs directly.
	directErr := runFFmpeg([]string{
		"-v", "error",
		"-y",
		"-i", filePath,
		"-vf", "fps=24,scale=512:512:flags=lanczos:force_original_aspect_ratio=decrease",
		"-loop", "0",
		outputFilePath,
	})
	if directErr == nil {
		return outputFilePath, nil
	}

	// Fallback for ffmpeg builds that require lottie JSON input.
	jsonFilePath := strings.TrimSuffix(filePath, ".tgs") + ".json"
	if err := extractTgsJSON(filePath, jsonFilePath); err != nil {
		return "", fmt.Errorf("failed to extract tgs json: %w", err)
	}
	defer os.Remove(jsonFilePath)

	jsonErr := runFFmpeg([]string{
		"-v", "error",
		"-y",
		"-f", "lottie",
		"-i", jsonFilePath,
		"-vf", "fps=24,scale=512:512:flags=lanczos:force_original_aspect_ratio=decrease",
		"-loop", "0",
		outputFilePath,
	})
	if jsonErr != nil {
		if strings.Contains(strings.ToLower(jsonErr.Error()), "unknown input format: 'lottie'") {
			return "", fmt.Errorf("%w: %v", ErrTgsConversionUnsupported, jsonErr)
		}
		return "", fmt.Errorf("failed to convert tgs to gif: direct=%v; lottie=%v; hint=install ffmpeg with lottie support or set general.tgs_support=false", directErr, jsonErr)
	}

	return outputFilePath, nil
}

func extractTgsJSON(tgsPath string, jsonPath string) error {
	tgsFile, err := os.Open(tgsPath)
	if err != nil {
		return err
	}
	defer tgsFile.Close()

	gzipReader, err := gzip.NewReader(tgsFile)
	if err != nil {
		return err
	}
	defer gzipReader.Close()

	jsonFile, err := os.Create(jsonPath)
	if err != nil {
		return err
	}
	defer jsonFile.Close()

	_, err = io.Copy(jsonFile, gzipReader)
	return err
}

func runFFmpeg(args []string) error {
	cmd := exec.Command("ffmpeg", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			return err
		}
		return fmt.Errorf("%w: %s", err, msg)
	}
	return nil
}
