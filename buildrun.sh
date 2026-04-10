#!/bin/bash
if ! command -v go &> /dev/null
then
    echo "Go is not installed or not in PATH. Please install Go and try again."
    exit 1
fi
DateStamp=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
Version="InDev"
go build -o sticker_go -ldflags "-X github.com/libost/sticker_go/version.Version=$Version -X github.com/libost/sticker_go/version.BuildTime=$DateStamp" main.go
echo "Build completed: sticker_go (Version: $Version, Build Time: $DateStamp)"
nohup ./sticker_go > sticker_go.log 2>&1 &
echo "sticker_go is running in the background. Logs are being written to sticker_go.log