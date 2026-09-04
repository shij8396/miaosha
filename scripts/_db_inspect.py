# -*- coding: utf-8 -*-
"""临时调试：查询指定商品的订单表，核对真实建单情况"""
import sys
import pymysql

if sys.platform == 'win32':
    sys.stdout.reconfigure(encoding='utf-8', errors='replace')

conn = pymysql.connect(host='127.0.0.1', port=3306, user='root', password='123456', database='miaosha', charset='utf8mb4')
cur = conn.cursor()
cur.execute('SHOW TABLES')
tables = [r[0] for r in cur.fetchall()]
print('all tables:', tables)

# 找到订单表
order_tables = [t for t in tables if 'order' in t.lower()]
print('order tables:', order_tables)

for t in order_tables:
    cur.execute(f'DESCRIBE {t}')
    cols = [r[0] for r in cur.fetchall()]
    print(f'\n== {t} columns ==')
    print(cols)
    try:
        cur.execute(f'SELECT * FROM {t} WHERE product_id=24 ORDER BY id')
        rows = cur.fetchall()
        print(f'rows for product 24: {len(rows)}')
        for r in rows:
            print(r)
    except Exception as e:
        print('query err:', e)

conn.close()
