FROM golang:1.25-alpine AS builder

WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/sticker_go .

FROM alpine:3.21

RUN apk add --no-cache ca-certificates docker-cli ffmpeg tzdata && update-ca-certificates

ENV IN_DOCKER=true

WORKDIR /data

COPY --from=builder /out/sticker_go /usr/local/bin/sticker_go

VOLUME ["/data"]

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/sticker_go"]
CMD ["-d", "/data"]