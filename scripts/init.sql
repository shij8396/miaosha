-- [修复] 去掉 CREATE DATABASE 和 USE 语句，依赖 Docker MYSQL_DATABASE 环境变量自动创建数据库
-- [修复] t_user 添加 role 字段，修复测试用户密码哈希，订单分表添加索引

CREATE TABLE IF NOT EXISTS t_user (
    id BIGINT NOT NULL AUTO_INCREMENT COMMENT '用户ID',
    username VARCHAR(64) NOT NULL COMMENT '用户名',
    password VARCHAR(256) NOT NULL COMMENT '密码',
    nickname VARCHAR(128) DEFAULT '' COMMENT '昵称',
    phone VARCHAR(20) DEFAULT '' COMMENT '手机号',
    email VARCHAR(128) DEFAULT '' COMMENT '邮箱',
    -- [修复] 添加 role 字段，支持 admin/user 角色区分
    role VARCHAR(32) DEFAULT 'user' COMMENT '角色admin/user',
    status TINYINT DEFAULT 1 COMMENT '状态',
    created_at DATETIME(3) DEFAULT NULL,
    updated_at DATETIME(3) DEFAULT NULL,
    deleted_at DATETIME(3) DEFAULT NULL,
    PRIMARY KEY (id), UNIQUE KEY idx_username (username), KEY idx_deleted_at (deleted_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS t_product (
    id BIGINT NOT NULL AUTO_INCREMENT COMMENT '商品ID',
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
    created_at DATETIME(3) DEFAULT NULL,
    updated_at DATETIME(3) DEFAULT NULL,
    deleted_at DATETIME(3) DEFAULT NULL,
    PRIMARY KEY (id), KEY idx_deleted_at (deleted_at), KEY idx_status_time (status, start_time, end_time)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

DELIMITER $$
CREATE PROCEDURE create_order_shard_tables()
BEGIN
    DECLARE i INT DEFAULT 0;
    WHILE i < 16 DO
        SET @sql = CONCAT('CREATE TABLE IF NOT EXISTS t_order_', i, ' (
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
            -- [修复] 订单分表添加 product_id 和 status 索引，优化按商品和状态查询
            KEY idx_product_id (product_id), KEY idx_status (status)
        ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4');
        PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;
        SET i = i + 1;
    END WHILE;
END$$
DELIMITER ;
CALL create_order_shard_tables();
DROP PROCEDURE IF EXISTS create_order_shard_tables;

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

-- [修复] 审计日志表，记录所有后台操作变更
CREATE TABLE IF NOT EXISTS t_audit_log (
    id BIGINT AUTO_INCREMENT PRIMARY KEY COMMENT '日志ID',
    user_id BIGINT NOT NULL COMMENT '操作用户ID',
    username VARCHAR(64) NOT NULL COMMENT '操作用户名',
    action VARCHAR(64) NOT NULL COMMENT '操作类型(create/update/delete/export)',
    module VARCHAR(64) NOT NULL COMMENT '操作模块(product/order/user/sentinel)',
    target_id VARCHAR(128) COMMENT '操作目标ID',
    detail TEXT COMMENT '操作详情JSON',
    client_ip VARCHAR(64) COMMENT '客户端IP',
    trace_id VARCHAR(128) COMMENT 'TraceID',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP COMMENT '操作时间',
    INDEX idx_user_id (user_id),
    INDEX idx_username (username),
    INDEX idx_action (action),
    INDEX idx_created_at (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='操作审计日志表';

-- [修复] 测试用户密码使用正确的 bcrypt 哈希值（$2a$10$... 对应密码 123456）
INSERT INTO t_user (username, password, nickname, role, status, created_at, updated_at) VALUES
('testuser', '$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy', '测试用户', 'user', 1, NOW(), NOW()),
('admin', '$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy', '管理员', 'admin', 1, NOW(), NOW());

INSERT INTO t_product (name, description, price, seckill_price, total_stock, remain_stock, start_time, end_time, status, limit_per_user, created_at, updated_at) VALUES
('iPhone 16 Pro Max', '苹果旗舰手机', 9999.00, 6999.00, 1000, 1000, '2026-01-01 00:00:00', '2027-12-31 23:59:59', 1, 1, NOW(), NOW()),
('MacBook Pro 16寸', '苹果笔记本电脑', 19999.00, 14999.00, 500, 500, '2026-01-01 00:00:00', '2027-12-31 23:59:59', 1, 1, NOW(), NOW()),
('AirPods Pro 3', '苹果降噪耳机', 1999.00, 1299.00, 2000, 2000, '2026-01-01 00:00:00', '2027-12-31 23:59:59', 1, 2, NOW(), NOW());