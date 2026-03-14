package utils

import (
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

func DecodeTgsToGIF(dir string) error {
	absPath, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	args := []string{
		"run", "--rm",
		"-v", fmt.Sprintf("%s:/source", absPath),
		"edasriyan/lottie-to-gif",
	}
	cmd := exec.Command("docker", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		trimmedOutput := strings.TrimSpace(string(output))
		lowerOutput := strings.ToLower(trimmedOutput)
		if strings.Contains(lowerOutput, "unsupported") || strings.Contains(lowerOutput, "not supported") {
			return fmt.Errorf("%w: %s", ErrTgsConversionUnsupported, trimmedOutput)
		}
		if trimmedOutput == "" {
			return fmt.Errorf("failed to run docker command: %w", err)
		}
		return fmt.Errorf("failed to run docker command: %w, output: %s", err, trimmedOutput)
	}

	return nil
}

func ExtractTgsJSON(fileid string) error {
	tgsFile, err := os.Open(C.CacheDir + fileid + ".tgs")
	if err != nil {
		return err
	}
	defer tgsFile.Close()

	gzipReader, err := gzip.NewReader(tgsFile)
	if err != nil {
		return err
	}
	defer gzipReader.Close()

	jsonFile, err := os.Create(C.CacheDir + fileid + ".json")
	if err != nil {
		return err
	}
	defer jsonFile.Close()

	_, err = io.Copy(jsonFile, gzipReader)
	return err
}
