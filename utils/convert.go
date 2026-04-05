package utils

import (
	"encoding/json"
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
	mountSourcePath := resolveDockerMountSourcePath(absPath)
	args := []string{
		"run", "--rm",
		"-v", fmt.Sprintf("%s:/source", mountSourcePath),
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

type dockerMount struct {
	Source      string `json:"Source"`
	Destination string `json:"Destination"`
}

func resolveDockerMountSourcePath(absContainerPath string) string {
	if os.Getenv("IN_DOCKER") != "true" {
		return absContainerPath
	}

	containerRef := strings.TrimSpace(os.Getenv("HOSTNAME"))
	if containerRef == "" {
		return absContainerPath
	}

	cmd := exec.Command("docker", "inspect", containerRef, "--format", "{{json .Mounts}}")
	output, err := cmd.Output()
	if err != nil {
		return absContainerPath
	}

	var mounts []dockerMount
	if err := json.Unmarshal(output, &mounts); err != nil {
		return absContainerPath
	}

	cleanAbs := filepath.Clean(absContainerPath)
	bestDest := ""
	bestSrc := ""
	for _, m := range mounts {
		dest := filepath.Clean(m.Destination)
		if dest == "." || m.Source == "" {
			continue
		}
		if !pathHasPrefix(cleanAbs, dest) {
			continue
		}
		if len(dest) > len(bestDest) {
			bestDest = dest
			bestSrc = filepath.Clean(m.Source)
		}
	}

	if bestDest == "" || bestSrc == "" {
		return absContainerPath
	}

	rel, err := filepath.Rel(bestDest, cleanAbs)
	if err != nil {
		return absContainerPath
	}
	if rel == "." {
		return bestSrc
	}

	return filepath.Clean(filepath.Join(bestSrc, rel))
}

func pathHasPrefix(path, prefix string) bool {
	path = filepath.Clean(path)
	prefix = filepath.Clean(prefix)
	if path == prefix {
		return true
	}
	if prefix == string(filepath.Separator) {
		return strings.HasPrefix(path, prefix)
	}
	return strings.HasPrefix(path, prefix+string(filepath.Separator))
}
