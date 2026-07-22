# [修复] 基础镜像改为 golang:1.24-alpine（可用版本），添加 Docker 配置覆盖
FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o miaosha-server ./cmd/
FROM alpine:3.19
RUN apk --no-cache add ca-certificates tzdata
ENV TZ=Asia/Shanghai
WORKDIR /app
COPY --from=builder /app/miaosha-server .
# [修复] 使用 Docker 专用配置覆盖本地开发配置
COPY --from=builder /app/config/config.docker.yaml ./config/config.yaml
EXPOSE 8080 9090
CMD ["./miaosha-server"]