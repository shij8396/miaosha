# -*- coding: utf-8 -*-
# M2 压测后一致性核对：MySQL 商品库存 + 关联订单数 vs 压测工具报告的消耗
import pymysql

conn = pymysql.connect(host='127.0.0.1', port=3306, user='root',
                       password='123456', database='miaosha', charset='utf8mb4')
cur = conn.cursor()

cur.execute("SELECT id, name, total_stock, remain_stock, status FROM t_product WHERE id=28")
print('product28(MySQL):', cur.fetchall())

total = 0
statuses = {}
for i in range(16):
    cur.execute("SHOW COLUMNS FROM t_order_%d LIKE 'product_id'" % i)
    if not cur.fetchall():
        continue
    cur.execute("SELECT COUNT(*) FROM t_order_%d WHERE product_id=28" % i)
    total += cur.fetchone()[0]
    cur.execute("SELECT status, COUNT(*) FROM t_order_%d WHERE product_id=28 GROUP BY status" % i)
    for s, c in cur.fetchall():
        statuses[str(s)] = statuses.get(str(s), 0) + c

print('order total(product28):', total)
print('by status:', statuses)
conn.close()
