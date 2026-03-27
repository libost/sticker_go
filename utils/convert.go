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
	filter := "fps=30,scale=512:-1:flags=lanczos,split[s0][s1];[s0]palettegen=reserve_transparent=on:transparency_color=ffffff[p];[s1][p]paletteuse=alpha_threshold=128"
	outputPath := strings.TrimSuffix(inputPath, ".webm") + ".gif"
	cmd := exec.Command("ffmpeg", "-i", inputPath, "-vf", filter, outputPath)
	err = cmd.Run()
	if err != nil {
		return "", err
	}
	return outputPath, nil
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
