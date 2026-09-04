#!/bin/bash
# ======================================================================
# 秒杀系统一键部署脚本（服务器端执行）
# ======================================================================
set -e
cd /opt/miaosha

echo "=========================================="
echo " 1. 检查 docker 网络名"
echo "=========================================="
NET_NAME=$(docker network ls --format '{{.Name}}' | grep miaosha | head -1)
echo "检测到网络: $NET_NAME"
if [ -z "$NET_NAME" ]; then
    echo "错误: 找不到 miaosha 网络"
    exit 1
fi

echo ""
echo "=========================================="
echo " 2. 修复配置文件 YAML 中的环境变量占位符"
echo "=========================================="
# 直接把 ${VAR} 占位符替换成实际值（容器内 Go 配置解析器可能不展开 env）
source /opt/miaosha/.env
cp config/config.prod.yaml config/config.prod.yaml.bak
sed -i "s|\${MIAOSHA_SERVER_SIGN_SECRET}|$MIAOSHA_SERVER_SIGN_SECRET|g" config/config.prod.yaml
sed -i "s|\${MIAOSHA_JWT_SECRET}|$MIAOSHA_JWT_SECRET|g" config/config.prod.yaml
sed -i "s|\${MIAOSHA_MYSQL_HOST}|$MIAOSHA_MYSQL_HOST|g" config/config.prod.yaml
sed -i "s|\${MIAOSHA_MYSQL_USER}|$MIAOSHA_MYSQL_USER|g" config/config.prod.yaml
sed -i "s|\${MIAOSHA_MYSQL_PASSWORD}|$MIAOSHA_MYSQL_PASSWORD|g" config/config.prod.yaml
sed -i "s|\${MIAOSHA_REDIS_HOST}|$MIAOSHA_REDIS_HOST|g" config/config.prod.yaml
sed -i "s|\${MIAOSHA_REDIS_PASSWORD}|$MIAOSHA_REDIS_PASSWORD|g" config/config.prod.yaml

# 替换 rabbitmq、etcd、kafka 的占位符（如果 YAML 里有）
sed -i "s|\${RABBITMQ_USER}|$RABBITMQ_USER|g" config/config.prod.yaml
sed -i "s|\${RABBITMQ_PASSWORD}|$RABBITMQ_PASSWORD|g" config/config.prod.yaml

# 检查是否还有漏网的 ${...}
REMAIN=$(grep -n '\${' config/config.prod.yaml || true)
if [ -n "$REMAIN" ]; then
    echo "警告: 还有未替换的占位符:"
    echo "$REMAIN"
else
    echo "配置文件占位符全部替换完成 ✓"
fi

echo ""
echo "=========================================="
echo " 3. 停止并删除旧的 miaosha-server 容器"
echo "=========================================="
docker compose -f docker-compose.prod.yml --env-file .env rm -sf miaosha-server 2>/dev/null || true

echo ""
echo "=========================================="
echo " 4. 确认数据库表存在(MySQL容器自动初始化了 init.sql)"
echo "=========================================="
sleep 3
# 用 docker exec 在 mysql 容器里查表
MYSQL_PWD="$MIAOSHA_MYSQL_PASSWORD" docker exec miaosha-mysql mysql -uroot -p"$MIAOSHA_MYSQL_PASSWORD" -e "SHOW TABLES FROM miaosha;" 2>&1 || true

echo ""
echo "=========================================="
echo " 5. 启动 miaosha-server 容器"
echo "=========================================="
docker compose -f docker-compose.prod.yml --env-file .env up -d miaosha-server 2>&1
sleep 1
docker ps --format 'table {{.Names}}\t{{.Status}}\t{{.Ports}}' | grep miaosha

echo ""
echo "=========================================="
echo " 6. 等待后端启动 20 秒,查看启动日志"
echo "=========================================="
sleep 20
echo ""
echo "--- 最后 40 行日志 (去重后的配置加载日志) ---"
docker logs miaosha-server --tail 60 2>&1 | awk '!/已加载配置文件/ || !seen[$0]++' | tail -40

echo ""
echo "=========================================="
echo " 7. 健康检查测试"
echo "=========================================="
HCHECK=$(curl -s --max-time 10 http://localhost:8080/health?type=readiness || echo "FAIL")
echo "健康检查返回: $HCHECK"

METRICS=$(curl -s --max-time 5 http://localhost:8080/metrics | head -5 || echo "FAIL")
echo "Metrics 探测: $METRICS"

echo ""
echo "=========================================="
echo " 8. 安装 Nginx"
echo "=========================================="
if ! command -v nginx >/dev/null 2>&1; then
    echo "开始安装 Nginx..."
    yum install -y epel-release 2>/dev/null || true
    yum install -y nginx
else
    echo "Nginx 已安装: $(nginx -v 2>&1)"
fi

echo ""
echo "=========================================="
echo " 9. 部署前端 dist + 配置 Nginx 反向代理"
echo "=========================================="
# 拷贝前端静态文件
NGINX_HTML="/usr/share/nginx/html/miaosha"
mkdir -p "$NGINX_HTML"
rm -rf "$NGINX_HTML"/*
cp -rf /opt/miaosha/dist/* "$NGINX_HTML"/
echo "前端文件已拷贝到: $NGINX_HTML ($(ls "$NGINX_HTML" | wc -l) 个文件/目录)"

# 写入 Nginx 配置
cat > /etc/nginx/conf.d/miaosha.conf << 'NGX_EOF'
server {
    listen       80;
    server_name  _;
    client_max_body_size 20m;

    # 安全头
    add_header X-Content-Type-Options nosniff;
    add_header X-Frame-Options SAMEORIGIN;
    add_header X-XSS-Protection "1; mode=block";

    # 前端静态页面
    location / {
        root   /usr/share/nginx/html/miaosha;
        index  index.html;
        try_files $uri $uri/ /index.html;
        expires 7d;
    }

    # 后端 API
    location /api/ {
        proxy_pass http://127.0.0.1:8080/api/;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_read_timeout 60s;
        proxy_send_timeout 60s;
    }

    # WebSocket (用于大屏 etc.)
    location /ws/ {
        proxy_pass http://127.0.0.1:8080/ws/;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_read_timeout 3600s;
    }

    # Prometheus 指标接口
    location /metrics {
        proxy_pass http://127.0.0.1:8080/metrics;
        allow 127.0.0.1;
        allow all;
    }

    # 健康检查
    location = /health {
        proxy_pass http://127.0.0.1:8080/health;
    }

    # 禁止访问隐藏文件
    location ~ /\. { deny all; }
}
NGX_EOF

echo "Nginx 配置写入完成"
nginx -t 2>&1 || true
systemctl restart nginx 2>&1
systemctl enable nginx 2>/dev/null || true

echo ""
echo "=========================================="
echo " 10. 开放防火墙端口"
echo "=========================================="
# firewalld (CentOS 默认)
if systemctl is-active --quiet firewalld 2>/dev/null; then
    firewall-cmd --permanent --add-port=80/tcp        # Nginx 前端/API
    firewall-cmd --permanent --add-port=8080/tcp      # 后端直连(备用)
    firewall-cmd --permanent --add-port=3000/tcp      # Grafana
    firewall-cmd --permanent --add-port=15672/tcp     # RabbitMQ UI
    firewall-cmd --permanent --add-port=9091/tcp      # Prometheus
    firewall-cmd --reload
    echo "firewalld 端口已开放 ✓"
elif command -v iptables >/dev/null 2>&1; then
    iptables -I INPUT -p tcp --dport 80 -j ACCEPT
    iptables -I INPUT -p tcp --dport 8080 -j ACCEPT
    iptables -I INPUT -p tcp --dport 3000 -j ACCEPT
    iptables -I INPUT -p tcp --dport 15672 -j ACCEPT
    iptables -I INPUT -p tcp --dport 9091 -j ACCEPT
    service iptables save 2>/dev/null || true
    echo "iptables 端口已开放 ✓"
else
    echo "未检测到 firewalld/iptables，如使用云安全组请在腾讯云控制台手动放行：80, 8080, 3000, 15672, 9091"
fi

echo ""
echo "=========================================="
echo " ★ 部署完成 —— 访问地址汇总 ★"
echo "=========================================="
IP=$(hostname -I | awk '{print $1}')
echo " 秒杀首页:        http://$IP/"
echo " 后端健康检查:    http://$IP/health"
echo " RabbitMQ UI:    http://$IP:15672  (admin / $RABBITMQ_PASSWORD)"
echo " Grafana 监控:   http://$IP:3000   (admin / $GRAFANA_PASSWORD)"
echo " Prometheus:     http://$IP:9091"
echo ""
echo "默认管理员账号:  admin / admin123  (登录页 /admin)"
echo "=========================================="
echo "如果后端没起来，请查看: docker logs miaosha-server --tail 100 2>&1"
echo ""
