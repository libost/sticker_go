package utils

import (
	"image/png"
	"os"
	"os/exec"

	C "libost/sticker_go/constants"

	"golang.org/x/image/webp"
)

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

/*
func DecodeTgsToGif(fileId string) (filePath string, err error) {
	// 这里需要使用第三方库来处理 TGS 动画贴纸并转换为 GIF
	// 例如，可以使用 lottie-web 或者其他工具来完成这个任务
	// 1. 准备 FFmpeg 命令，设置输入为管道
	cmd := exec.Command("ffmpeg",
    	"-f", "image2pipe",     // 输入格式为图像流
    	"-vcodec", "png",       // 或者 rawvideo
    	"-i", "-",              // 从 Stdin 读取
    	"-vf", "split[s0][s1];[s0]palettegen[p];[s1][p]paletteuse", // FFmpeg 经典的生成高质量 GIF 的滤镜
    	C.CacheDir+fileId+".gif",
	)

	stdin, _ := cmd.StdinPipe()
	cmd.Start()

	// 2. 循环渲染帧并写入管道
	for i := 0; i < totalFrames; i++ {
    	rgbaImg := renderFrame(i) // 使用 rlottie 渲染一帧
    	png.Encode(stdin, rgbaImg) // 将帧以 PNG 格式写入 FFmpeg 管道
	}

	stdin.Close()
	cmd.Wait()
}*/
