package utils

import (
	"errors"
	"fmt"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/image/webp"
)

var ErrTgsConversionUnsupported = errors.New("tgs conversion unsupported by current Docker-based converter")

func DecodeWebPToPNG(inputPath string) (filePath string, err error) {
	f, err := os.Open(inputPath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	img, err := webp.Decode(f)
	if err != nil {
		return "", err
	}
	outPutFile, err := os.Create(strings.TrimSuffix(inputPath, ".webp") + ".png")
	if err != nil {
		return "", err
	}
	defer outPutFile.Close()
	err = png.Encode(outPutFile, img)
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(inputPath, ".webp") + ".png", nil
}

func DecodeWebMToGIF(inputPath string) (filePath string, err error) {
	// 这里需要使用第三方库来处理 WebM 视频并转换为 GIF
	// 例如，可以使用 ffmpeg 命令行工具来完成这个任务
	// 你需要确保系统上安装了 ffmpeg，并且在 PATH 中可用
	cmd := exec.Command("ffmpeg", "-i", inputPath, strings.TrimSuffix(inputPath, ".webm")+".gif")
	err = cmd.Run()
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(inputPath, ".webm") + ".gif", nil
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
