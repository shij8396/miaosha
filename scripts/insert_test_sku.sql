INSERT INTO t_product_sku (product_id, spec, price, stock, status, created_at, updated_at) VALUES
(20, '{"颜色":"黑","存储":"256G"}', 11.00, 50, 1, NOW(), NOW()),
(20, '{"颜色":"白","存储":"256G"}', 12.00, 50, 1, NOW(), NOW()),
(20, '{"颜色":"黑","存储":"512G"}', 13.00, 50, 1, NOW(), NOW()),
(20, '{"颜色":"白","存储":"512G"}', 14.00, 50, 1, NOW(), NOW());
SELECT id, product_id, spec, price, stock FROM t_product_sku WHERE product_id = 20 AND deleted_at IS NULL;
