#!/bin/bash
# ========== 秒杀系统生产环境一键部署脚本 ==========
# 在服务器 /opt/miaosha 目录下执行
set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}  秒杀系统 生产环境部署脚本${NC}"
echo -e "${GREEN}========================================${NC}"

# 检查必要文件
if [ ! -f .env.production ]; then
    echo -e "${RED}错误：.env.production 文件不存在${NC}"
    echo "请先复制模板: cp .env.production .env"
    echo "然后编辑 .env 填入你的密码"
    exit 1
fi

# 检查 .env 是否已配置密码
if grep -q "请改成" .env.production 2>/dev/null || grep -q "你的" .env.production 2>/dev/null; then
    echo -e "${YELLOW}警告：.env.production 中还有默认占位符${NC}"
    echo "请修改 .env.production 中的密码后再继续"
    echo -e "${YELLOW}当前文件内容：${NC}"
    cat .env.production
    read -p "是否继续？(y/N): " confirm
    if [ "$confirm" != "y" ]; then
        exit 1
    fi
fi

# 加载环境变量
cp .env.production .env
source .env

echo -e "${GREEN}[1/6] 准备目录结构...${NC}"
mkdir -p logs uploads
mkdir -p /opt/miaosha/uploads
chmod -R 755 logs uploads

echo -e "${GREEN}[2/6] 检查并初始化 MySQL 数据库...${NC}"
# 使用已有 MySQL，创建数据库和表
mysql -u${MIAOSHA_MYSQL_USER} -p${MIAOSHA_MYSQL_PASSWORD} -e "
CREATE DATABASE IF NOT EXISTS miaosha DEFAULT CHARSET utf8mb4 COLLATE utf8mb4_unicode_ci;
USE miaosha;
SOURCE scripts/init.sql;
" 2>/dev/null || {
    echo -e "${YELLOW}MySQL 命令执行失败，可能密码不对或 MySQL 未启动${NC}"
    echo "请手动执行："
    echo "  mysql -u${MIAOSHA_MYSQL_USER} -p < scripts/init.sql"
    echo "然后创建数据库："
    echo "  CREATE DATABASE miaosha DEFAULT CHARSET utf8mb4;"
    echo "  USE miaosha;"
    echo "  SOURCE scripts/init.sql;"
}

echo -e "${GREEN}[3/6] 构建前端...${NC}"
cd frontend
if [ ! -d node_modules ]; then
    npm install --registry https://registry.npmmirror.com
fi
npm run build
cd ..
echo "前端构建完成: frontend/dist/"

echo -e "${GREEN}[4/6] 构建后端 Docker 镜像...${NC}"
docker compose -f docker-compose.prod.yml build miaosha-server

echo -e "${GREEN}[5/6] 启动所有服务...${NC}"
docker compose -f docker-compose.prod.yml up -d

echo -e "${GREEN}[6/6] 等待服务就绪（约 30 秒）...${NC}"
sleep 30

# 检查服务状态
echo ""
echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}  部署完成！服务状态：${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""

docker compose -f docker-compose.prod.yml ps

echo ""
echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}  访问地址${NC}"
echo -e "${GREEN}========================================${NC}"
echo "  前端页面:  http://服务器IP"
echo "  Grafana:   http://服务器IP:3000  (密码: ${GRAFANA_PASSWORD})"
echo "  Prometheus: http://服务器IP:9091"
echo "  RabbitMQ:  http://服务器IP:15672  (密码: ${RABBITMQ_PASSWORD})"
echo ""
echo "  默认账号: admin / test123"
echo ""
echo -e "${YELLOW}重要：接下来需要配置 Nginx 反代${NC}"
echo "  执行: cp deploy/nginx.prod.conf /etc/nginx/conf.d/miaosha.conf"
echo "  执行: nginx -t && systemctl reload nginx"
