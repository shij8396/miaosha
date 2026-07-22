@echo off
chcp 65001 >nul
title 秒杀系统一键启动脚本

echo ============================================
echo   分布式秒杀系统 - 一键启动 (Windows)
echo ============================================
echo.

REM 检查 Docker 是否运行
docker info >nul 2>&1
if errorlevel 1 (
    echo [错误] Docker 未运行，请先启动 Docker Desktop
    pause
    exit /b 1
)

REM 启动中间件集群
echo [1/4] 启动中间件集群 (MySQL/Redis/RabbitMQ/Kafka/Etcd)...
docker-compose -f docker-compose.yml up -d
echo [1/4] 中间件集群启动中，等待健康检查...
timeout /t 10 /nobreak >nul

REM 编译 Go 后端
echo [2/4] 编译 Go 后端服务...
cd /d "%~dp0"
go build -o miaosha.exe ./cmd/
if errorlevel 1 (
    echo [错误] 后端编译失败
    pause
    exit /b 1
)
echo [2/4] 后端编译成功

REM 编译前端
echo [3/4] 编译前端页面...
cd frontend
call npm run build
if errorlevel 1 (
    echo [错误] 前端编译失败
    pause
    exit /b 1
)
cd ..

REM 启动后端服务
echo [4/4] 启动后端服务...
start "秒杀系统-后端" miaosha.exe

echo.
echo ============================================
echo   启动完成！
echo   后端地址: http://localhost:8080
echo   健康检查: http://localhost:8080/health
echo   前端页面: http://localhost:8080
echo   Prometheus: http://localhost:9090
echo ============================================
echo.
echo 按任意键关闭窗口...
pause >nul