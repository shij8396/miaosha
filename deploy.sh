#!/bin/bash
set -e

echo ""
echo "╔══════════════════════════════════════════════╗"
echo "║     分布式秒杀系统 - 一键部署 (Docker)        ║"
echo "╚══════════════════════════════════════════════╝"
echo ""

# 检查 Docker
echo "[1/4] 检查 Docker 环境..."
if ! docker info > /dev/null 2>&1; then
    echo "[X] 未检测到 Docker，请先安装 Docker"
    echo "    下载地址: https://www.docker.com/products/docker-desktop"
    exit 1
fi
echo "[√] Docker 已就绪"

# 检查 docker compose
echo "[2/4] 检查 docker compose..."
if ! docker compose version > /dev/null 2>&1; then
    echo "[X] docker compose 命令不可用，请更新 Docker"
    exit 1
fi
echo "[√] docker compose 已就绪"

# 启动所有服务
echo "[3/4] 启动服务集群 (首次需拉取镜像，约 5-10 分钟)..."
docker compose up -d

# 等待服务就绪
echo "[4/4] 等待服务就绪..."
echo "正在检查 MySQL 主库..."
until docker exec miaosha-mysql-master mysqladmin ping -h localhost -u root -pmiaosha_master_2026 --silent > /dev/null 2>&1; do
    sleep 5
done
echo "[√] MySQL 主库就绪"

echo "正在检查后端服务..."
until curl -s http://localhost/health > /dev/null 2>&1; do
    sleep 5
done
echo "[√] 后端服务就绪"

echo ""
echo "╔══════════════════════════════════════════════╗"
echo "║              部署完成！请访问：               ║"
echo "╠══════════════════════════════════════════════╣"
echo "║  前端页面:    http://localhost               ║"
echo "║  Grafana:     http://localhost:3000          ║"
echo "║  RabbitMQ:    http://localhost:15672         ║"
echo "║  Prometheus:  http://localhost:9091          ║"
echo "╠══════════════════════════════════════════════╣"
echo "║  测试账号:                                   ║"
echo "║  管理员: admin    / test123                  ║"
echo "║  普通用户: testuser / test123                ║"
echo "╚══════════════════════════════════════════════╝"
echo ""