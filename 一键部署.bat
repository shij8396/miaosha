@echo off
chcp 65001 >nul
title 秒杀系统 - 一键部署

echo.
echo ╔══════════════════════════════════════════════╗
echo ║     分布式秒杀系统 - 一键部署 (Docker)        ║
echo ╚══════════════════════════════════════════════╝
echo.

REM 检查 Docker
echo [1/4] 检查 Docker 环境...
docker info >nul 2>&1
if errorlevel 1 (
    echo [X] 未检测到 Docker，请先安装 Docker Desktop
    echo     下载地址: https://www.docker.com/products/docker-desktop
    pause
    exit /b 1
)
echo [√] Docker 已就绪

REM 检查 docker-compose
echo [2/4] 检查 docker-compose...
docker compose version >nul 2>&1
if errorlevel 1 (
    echo [X] docker compose 命令不可用，请更新 Docker Desktop
    pause
    exit /b 1
)
echo [√] docker compose 已就绪

REM 启动所有服务
echo [3/4] 启动服务集群 (首次需拉取镜像，约 5-10 分钟)...
docker compose up -d
if errorlevel 1 (
    echo [X] 启动失败，请检查错误信息
    pause
    exit /b 1
)

REM 等待服务就绪
echo [4/4] 等待服务就绪...
echo 正在检查 MySQL 主库...
:wait_mysql
timeout /t 5 /nobreak >nul
docker exec miaosha-mysql-master mysqladmin ping -h localhost -u root -pmiaosha_master_2026 --silent >nul 2>&1
if errorlevel 1 goto wait_mysql
echo [√] MySQL 主库就绪

echo 正在检查后端服务...
:wait_server
timeout /t 5 /nobreak >nul
curl -s http://localhost/health >nul 2>&1
if errorlevel 1 goto wait_server
echo [√] 后端服务就绪

echo.
echo ╔══════════════════════════════════════════════╗
echo ║              部署完成！请访问：               ║
echo ╠══════════════════════════════════════════════╣
echo ║  前端页面:    http://localhost               ║
echo ║  Grafana:     http://localhost:3000          ║
echo ║  RabbitMQ:    http://localhost:15672         ║
echo ║  Prometheus:  http://localhost:9091          ║
echo ╠══════════════════════════════════════════════╣
echo ║  测试账号:                                   ║
echo ║  管理员: admin    / test123                  ║
echo ║  普通用户: testuser / test123                ║
echo ╚══════════════════════════════════════════════╝
echo.
echo 按任意键关闭窗口...
pause >nul