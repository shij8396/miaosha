#!/bin/bash
# ============================================================
# 秒杀系统终极修复脚本（one-shot）
# 目标：把服务器上现存的「库不存在 / root@% 没权限 / 后端崩溃 / 密码错 / 签名密钥不一致 / 限流残留」全部修好，
#       最后给出 admin/test123 三端登录验证结果。
# 用法：  bash fix-all.sh
# ============================================================
set +e  # 单步失败不中断脚本，保证把所有诊断信息都打印出来
cd /opt/miaosha
trap 'echo -e "\n===== EXIT_CODE=$? 脚本执行结束，把上面全部输出贴给我。====="' EXIT

echo -e "\n\n##################################################"
echo -e "# 【1/7】数据库：授权 + 建库 + 导入 init.sql"
echo -e "##################################################"

echo "== 1.1 给 root@'%' 授全权（修 1044 Access denied）=="
docker exec miaosha-mysql mysql -uroot -p123456 -e "
CREATE USER IF NOT EXISTS 'root'@'%' IDENTIFIED BY '123456';
GRANT ALL PRIVILEGES ON *.* TO 'root'@'%' WITH GRANT OPTION;
FLUSH PRIVILEGES;
SHOW DATABASES;" 2>&1 | tail -20

echo "== 1.2 建 miaosha 库 + 导入 init.sql（若 scripts/init.sql 不存在则内嵌 SQL 建表）=="
docker exec miaosha-mysql mysql -uroot -p123456 -e "CREATE DATABASE IF NOT EXISTS miaosha DEFAULT CHARSET utf8mb4 COLLATE utf8mb4_general_ci;"
if [ -f scripts/init.sql ]; then
  echo "· 存在 scripts/init.sql，使用它导入"
  docker exec -i miaosha-mysql mysql -uroot -p123456 miaosha < scripts/init.sql && echo "✓ init.sql 导入成功" || echo "⚠ init.sql 导入失败，改用内嵌 SQL"
fi
# 内嵌兜底 SQL（幂等 IF NOT EXISTS / IGNORE）
docker exec -i miaosha-mysql mysql -uroot -p123456 miaosha <<'SQL_END'
CREATE TABLE IF NOT EXISTS t_user (
    id BIGINT NOT NULL AUTO_INCREMENT,
    username VARCHAR(64) NOT NULL,
    password VARCHAR(256) NOT NULL,
    nickname VARCHAR(128) DEFAULT '',
    phone VARCHAR(20) DEFAULT '',
    email VARCHAR(128) DEFAULT '',
    role VARCHAR(32) DEFAULT 'user',
    status TINYINT DEFAULT 1,
    created_at DATETIME(3) DEFAULT NULL,
    updated_at DATETIME(3) DEFAULT NULL,
    deleted_at DATETIME(3) DEFAULT NULL,
    PRIMARY KEY (id), UNIQUE KEY idx_username (username), KEY idx_deleted_at (deleted_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS t_product (
    id BIGINT NOT NULL AUTO_INCREMENT,
    name VARCHAR(256) NOT NULL,
    description TEXT,
    price DECIMAL(10,2) NOT NULL,
    seckill_price DECIMAL(10,2) NOT NULL,
    total_stock INT NOT NULL,
    remain_stock INT NOT NULL,
    start_time DATETIME(3) NOT NULL,
    end_time DATETIME(3) NOT NULL,
    status TINYINT DEFAULT 0,
    image_url VARCHAR(512) DEFAULT '',
    limit_per_user INT DEFAULT 1,
    pay_timeout BIGINT DEFAULT 30,
    created_at DATETIME(3) DEFAULT NULL,
    updated_at DATETIME(3) DEFAULT NULL,
    deleted_at DATETIME(3) DEFAULT NULL,
    PRIMARY KEY (id), KEY idx_deleted_at (deleted_at), KEY idx_status_time (status, start_time, end_time)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 16 张订单分表
SET @i = 0;
WHILE @i < 16 DO
    SET @sql = CONCAT('CREATE TABLE IF NOT EXISTS t_order_', @i, ' (
        id BIGINT NOT NULL AUTO_INCREMENT,
        order_no VARCHAR(64) NOT NULL,
        user_id BIGINT NOT NULL,
        product_id BIGINT NOT NULL,
        product_name VARCHAR(256) NOT NULL,
        seckill_price DECIMAL(10,2) NOT NULL,
        quantity INT DEFAULT 1,
        total_amount DECIMAL(10,2) NOT NULL,
        status TINYINT DEFAULT 0,
        pay_time DATETIME(3) DEFAULT NULL,
        cancel_time DATETIME(3) DEFAULT NULL,
        cancel_reason VARCHAR(256) DEFAULT '''''',
        created_at DATETIME(3) DEFAULT NULL,
        updated_at DATETIME(3) DEFAULT NULL,
        PRIMARY KEY (id), UNIQUE KEY idx_order_no (order_no), KEY idx_user_id (user_id),
        KEY idx_product_id (product_id), KEY idx_status (status)
    ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4');
    PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;
    SET @i = @i + 1;
END WHILE;

CREATE TABLE IF NOT EXISTS t_recon_diff (
    id BIGINT NOT NULL AUTO_INCREMENT,
    product_id BIGINT NOT NULL,
    redis_stock INT NOT NULL,
    mysql_stock INT NOT NULL,
    diff INT NOT NULL,
    auto_corrected TINYINT DEFAULT 0,
    corrected_at DATETIME(3) DEFAULT NULL,
    created_at DATETIME(3) DEFAULT NULL,
    PRIMARY KEY (id), KEY idx_product_id (product_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS t_blacklist (
    id BIGINT NOT NULL AUTO_INCREMENT,
    type VARCHAR(32) NOT NULL,
    value VARCHAR(256) NOT NULL,
    reason VARCHAR(512) DEFAULT '',
    created_at DATETIME(3) DEFAULT NULL,
    PRIMARY KEY (id), UNIQUE KEY idx_t_blacklist_value (value), KEY idx_t_blacklist_type (type)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS t_audit_log (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    user_id BIGINT NOT NULL,
    username VARCHAR(64) NOT NULL,
    action VARCHAR(64) NOT NULL,
    module VARCHAR(64) NOT NULL,
    target_id VARCHAR(128),
    detail TEXT,
    client_ip VARCHAR(64),
    trace_id VARCHAR(128),
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_user_id (user_id), INDEX idx_username (username),
    INDEX idx_action (action), INDEX idx_created_at (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 测试账号/商品 幂等插入（忽略重复键）
INSERT IGNORE INTO t_user (username, password, nickname, role, status, created_at, updated_at) VALUES
('testuser', '$2a$12$uzVNfS7z2OS9BM3qIa2HeOZQKuYJk5N8yLz.f82IUs66Dh82sFYIm', '测试用户', 'user', 1, NOW(), NOW()),
('admin',    '$2a$12$uzVNfS7z2OS9BM3qIa2HeOZQKuYJk5N8yLz.f82IUs66Dh82sFYIm', '管理员',   'admin', 1, NOW(), NOW());

INSERT IGNORE INTO t_product (name, description, price, seckill_price, total_stock, remain_stock, start_time, end_time, status, limit_per_user, created_at, updated_at) VALUES
('iPhone 16 Pro Max', '苹果旗舰手机', 9999.00, 6999.00, 1000, 1000, '2026-01-01 00:00:00', '2027-12-31 23:59:59', 1, 1, NOW(), NOW()),
('MacBook Pro 16寸', '苹果笔记本电脑', 19999.00, 14999.00, 500, 500, '2026-01-01 00:00:00', '2027-12-31 23:59:59', 1, 1, NOW(), NOW()),
('AirPods Pro 3', '苹果降噪耳机', 1999.00, 1299.00, 2000, 2000, '2026-01-01 00:00:00', '2027-12-31 23:59:59', 1, 2, NOW(), NOW());
SQL_END

echo "== 1.3 数据库现状 =="
docker exec miaosha-mysql mysql -uroot -p123456 -t -e "
SELECT id,username,role,status,LEFT(password,20) hash_prefix FROM miaosha.t_user;
SELECT id,name,status,remain_stock FROM miaosha.t_product;
"

echo -e "\n\n##################################################"
echo -e "# 【2/7】确保 MySQL/Redis 容器接入后端网络（DNS 解析 mysql/redis 可用）"
echo -e "##################################################"
BACKEND_NET=$(docker inspect miaosha-server 2>/dev/null --format '{{range $k,$v := .NetworkSettings.Networks}}{{$k}}{{end}}' | head -1)
[ -z "$BACKEND_NET" ] && BACKEND_NET=$(docker network ls --format '{{.Name}}' | grep -E 'miaosha(-|_)miaosha-net' | head -1)
[ -z "$BACKEND_NET" ] && BACKEND_NET="miaosha_miaosha-net"
echo "后端所在网络: $BACKEND_NET"
for C in miaosha-mysql miaosha-redis; do
  docker network connect --alias "$(echo "$C" | sed 's/miaosha-//')" "$BACKEND_NET" "$C" 2>/dev/null \
    && echo "✓ $C 已接入 $BACKEND_NET（alias=$(echo "$C" | sed 's/miaosha-//')）" \
    || echo "· $C 已在 $BACKEND_NET（跳过）"
done

echo -e "\n\n##################################################"
echo -e "# 【3/7】配置文件：签名密钥对齐 + 日志输出 both（避免 console 没日志的排障盲区）"
echo -e "##################################################"
sed -i 's|^MIAOSHA_SERVER_SIGN_SECRET=.*|MIAOSHA_SERVER_SIGN_SECRET=miaosha-sign-secret-2026|' .env
sed -i 's|  sign_secret: "${MIAOSHA_SERVER_SIGN_SECRET}"|  sign_secret: "${MIAOSHA_SERVER_SIGN_SECRET}"|' config/config.prod.yaml
# 生产日志：console + file 双写（幂等替换）
grep -qE '^  output: "both"' config/config.prod.yaml \
  || sed -i 's|^  output: "file"|  output: "both"|' config/config.prod.yaml
echo ".env sign_secret 现在是:  $(grep MIAOSHA_SERVER_SIGN_SECRET .env)"
echo "config sign 输出 :  $(grep -E 'sign_secret|output:' config/config.prod.yaml | head -4)"

echo -e "\n\n##################################################"
echo -e "# 【4/7】重建后端（--no-deps，不被 kafka 健康卡脖子）+ 等待就绪"
echo -e "##################################################"
docker compose -f docker-compose.prod.yml --env-file .env up -d --no-deps --force-recreate miaosha-server
echo "等 18 秒后端启动..."
sleep 18
echo "后端容器状态: $(docker ps --filter name=miaosha-server --format '{{.Names}}: {{.Status}}')"
echo "---- 最近 25 行日志 ----"
docker logs --tail 25 miaosha-server

echo -e "\n\n##################################################"
echo -e "# 【5/7】用后端注册接口生成真实 test123 哈希 → 重置 admin/testuser（100% 同源哈希，杜绝误判）"
echo -e "##################################################"
TMPU="tmpfix$(date +%s)"
REG_OUT=$(curl -s -m 6 -X POST http://localhost:8080/api/v1/user/register \
  -H 'Content-Type: application/json' \
  -d "{\"username\":\"$TMPU\",\"password\":\"test123\",\"nickname\":\"tmp_fix\",\"phone\":\"13800000000\"}")
echo "注册临时用户返回: $REG_OUT"
HASH=$(docker exec miaosha-mysql mysql -uroot -p123456 -N -B -e "SELECT password FROM miaosha.t_user WHERE username='$TMPU' LIMIT 1;" 2>/dev/null)
if [ -n "$HASH" ]; then
  docker exec miaosha-mysql mysql -uroot -p123456 miaosha -e "UPDATE t_user SET password='$HASH',status=1 WHERE username IN ('admin','testuser'); DELETE FROM t_user WHERE username='$TMPU';"
  echo "✓ 已使用同源哈希重置 admin / testuser 密码为 test123（前缀: ${HASH:0:15}...）"
else
  echo "⚠ 取哈希失败，退回使用 init.sql 内置的 test123 bcrypt 哈希"
  FALLBACK='$2a$12$uzVNfS7z2OS9BM3qIa2HeOZQKuYJk5N8yLz.f82IUs66Dh82sFYIm'
  docker exec miaosha-mysql mysql -uroot -p123456 miaosha -e "UPDATE t_user SET password='$FALLBACK',status=1 WHERE username IN ('admin','testuser');"
fi
docker exec miaosha-mysql mysql -uroot -p123456 -t -e "SELECT id,username,role,status FROM miaosha.t_user;"

echo -e "\n\n##################################################"
echo -e "# 【6/7】清限流（rate_limit / 黑名单）"
echo -e "##################################################"
R=0; for k in $(docker exec miaosha-redis redis-cli --scan --pattern 'rate_limit:*' 2>/dev/null); do
  docker exec miaosha-redis redis-cli DEL "$k" >/dev/null; R=$((R+1))
done
echo "已清除限流 key 数量: $R"

echo -e "\n\n##################################################"
echo -e "# 【7/7】★ 三端登录验证 admin / test123 ★（成功标志：code=200 + token=eyJ...）"
echo -e "##################################################"
CHECK_LOGIN() {
  local NAME=$1; local URL=$2;
  local OUT; OUT=$(curl -s -m 6 -w "\nHTTP=%{http_code}" -X POST "$URL" -H 'Content-Type: application/json' \
    -d '{"username":"admin","password":"test123"}')
  local TOKEN=$(echo "$OUT" | grep -oE '"token":"[^"]+"' | head -1)
  local CODE=$(echo "$OUT" | grep -oE '"code":\s*[0-9]+' | head -1)
  local HTTP=$(echo "$OUT" | tail -1)
  if echo "$CODE" | grep -q "200" && [ -n "$TOKEN" ]; then
    echo -e "✓ [$NAME] 成功  HTTP=$HTTP  $CODE  token_prefix=$(echo "$TOKEN" | cut -c1-25)..."
  else
    echo -e "✗ [$NAME] 失败  HTTP=$HTTP  CODE=$CODE  响应前 120 字: $(echo "$OUT" | head -1 | cut -c1-120)"
  fi
}
CHECK_LOGIN "8080直连"   "http://localhost:8080/api/v1/user/login"
CHECK_LOGIN "Nginx80"    "http://localhost/api/v1/user/login"
CHECK_LOGIN "公网IP"     "http://115.159.157.18/api/v1/user/login"

echo -e "\n✳️  【健康检查】"
curl -s -m 5 -w " HTTP=%{http_code}\n" "http://localhost:8080/health"
curl -s -m 5 -w " HTTP=%{http_code}\n" "http://localhost/health"
