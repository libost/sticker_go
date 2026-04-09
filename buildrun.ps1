if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    Write-Error "Go is not installed or not in PATH. Please install Go and try again."
    exit 1
}
$DateStamp = Get-Date -Format "s"
$Version = "InDev"
go build -o sticker_go.exe -ldflags "-X github.com/libost/sticker_go/version.Version=$Version -X github.com/libost/sticker_go/version.BuildTime=$DateStamp -s -w" main.go
Write-Output "Build completed: sticker_go.exe (Version: $Version, Build Time: $DateStamp)"
./sticker_go.exe