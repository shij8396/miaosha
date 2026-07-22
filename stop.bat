@echo off
chcp 65001 >nul
title 秒杀系统一键停止脚本

echo ============================================
echo   分布式秒杀系统 - 一键停止 (Windows)
echo ============================================
echo.

REM 停止 Go 后端服务
echo [1/3] 停止后端服务...
taskkill /f /im miaosha.exe /t >nul 2>&1
echo [1/3] 后端服务已停止

REM 停止中间件集群
echo [2/3] 停止中间件集群...
cd /d "%~dp0"
docker-compose down
echo [2/3] 中间件集群已停止

echo [3/3] 清理完成
echo.
echo ============================================
echo   所有服务已停止
echo ============================================
echo.
echo 按任意键关闭窗口...
pause >nul