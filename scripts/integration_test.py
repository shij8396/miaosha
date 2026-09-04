# -*- coding: utf-8 -*-
"""
M2 集成测试 — 秒杀全链路真实依赖验证（本地 MySQL + Redis，RabbitMQ 降级为同步建单）
覆盖：验证码强制(含答案为0边界)/路径Token一次性/幂等/限购/超卖防护/支付回调/取消回滚/最终一致性

用法: python scripts/integration_test.py
依赖: 本地后端已启动 (127.0.0.1:8080)，MySQL+Redis 就绪
[M3] 管理员密码支持环境变量 ADMIN_PASSWORD 覆盖（默认 admin123，CI 注入 test123 与 init.sql 一致）
"""
import os
import sys
import json
import time
import hmac
import hashlib
import urllib.request
import urllib.error

if sys.platform == 'win32':
    sys.stdout.reconfigure(encoding='utf-8', errors='replace')

BASE = 'http://127.0.0.1:8080'
SECRET = 'miaosha-sign-secret-2026'
ADMIN_PASSWORD = os.environ.get('ADMIN_PASSWORD', 'admin123')

PASS = 0
FAIL = 0


def check(name, cond, detail=''):
    global PASS, FAIL
    if cond:
        PASS += 1
        print(f'  [PASS] {name} {detail}')
    else:
        FAIL += 1
        print(f'  [FAIL] {name} {detail}')


def api(method, path, token=None, body=None):
    url = BASE + path
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(url, data=data, method=method)
    req.add_header('Content-Type', 'application/json')
    if token:
        req.add_header('Authorization', 'Bearer ' + token)
    p = path.split('?')[0]
    body_str = json.dumps(body) if body is not None else ''
    ts = str(int(time.time()))
    sign = hmac.new(SECRET.encode(), (ts + method.upper() + p + body_str).encode(), hashlib.sha256).hexdigest()
    req.add_header('X-Timestamp', ts)
    req.add_header('X-Sign', sign)
    try:
        with urllib.request.urlopen(req, timeout=15) as r:
            return r.status, json.loads(r.read().decode())
    except urllib.error.HTTPError as e:
        try:
            return e.code, json.loads(e.read().decode())
        except Exception:
            return e.code, {}


def solve(expr):
    """解析数学算式 3+5=? / 5-5=? 返回答案（可为0）"""
    for op in ('+', '-'):
        if op in expr:
            a, b = expr.split(op)
            a, b = int(a.strip()), int(b.strip())
            return a + b if op == '+' else a - b
    raise ValueError(f'无法解析算式: {expr}')


def fresh_path_captcha(token, product_id):
    """获取新的 path_token + 验证码，返回 (path_token, captcha_id, answer)"""
    _, pt = api('GET', f'/api/v1/seckill/path?product_id={product_id}', token)
    ptok = pt['data']['path_token']
    _, cp = api('GET', f'/api/v1/seckill/captcha?product_id={product_id}', token)
    cid = cp['data']['captcha_id']
    ans = solve(cp['data']['expression'])
    return ptok, cid, ans


def do_seckill(token, product_id, path_token, captcha_id, captcha_code, idem):
    return api('POST', '/api/v1/seckill', token, {
        'product_id': product_id, 'quantity': 1, 'idempotent_key': idem,
        'path_token': path_token, 'captcha_id': captcha_id, 'captcha_code': captcha_code,
    })


def main():
    print('=' * 60)
    print(' M2 集成测试 — 秒杀全链路（本地 MySQL+Redis）')
    print('=' * 60)

    # ---------- 1. 账号准备 ----------
    print('\n[1] 账号准备')
    st, r = api('POST', '/api/v1/user/login', body={'username': 'admin', 'password': ADMIN_PASSWORD})
    admin_token = r.get('data', {}).get('token', '')
    check('管理员登录', st == 200 and admin_token, f'st={st}')

    tokens = {}
    for tag in ('A', 'B'):
        uname = f'it_{tag}_{int(time.time())}'
        api('POST', '/api/v1/user/register', body={'username': uname, 'password': 'test123'})
        st, r = api('POST', '/api/v1/user/login', body={'username': uname, 'password': 'test123'})
        tokens[tag] = r.get('data', {}).get('token', '')
        check(f'用户{tag}注册+登录', st == 200 and tokens[tag], f'st={st}')
    tA, tB = tokens['A'], tokens['B']

    # ---------- 2. 创建测试商品（库存5，限购2）并上架+预热 ----------
    print('\n[2] 测试商品准备（库存=5，限购=2）')
    now = time.strftime('%Y-%m-%d %H:%M:%S')
    end = time.strftime('%Y-%m-%d %H:%M:%S', time.localtime(time.time() + 86400))
    st, r = api('POST', '/api/v1/product', admin_token, {
        'name': f'集成测试商品_{int(time.time())}', 'price': 99.99, 'seckill_price': 9.99,
        'total_stock': 5, 'description': 'M2 integration test', 'limit_per_user': 2,
        'start_time': now, 'end_time': end,
    })
    pid = r.get('data', {}).get('id')
    check('创建商品', st == 200 and pid, f'pid={pid}')
    st, r = api('PUT', '/api/v1/activity', admin_token, {'product_id': pid, 'status': 1})
    check('上架秒杀活动', st == 200, f'msg={r.get("message")}')
    st, r = api('POST', '/api/v1/activity/config', admin_token,
                {'product_id': pid, 'limit_num': 2, 'start_time': now, 'end_time': end})
    check('配置活动限购=2', st == 200, f'msg={r.get("message")}')
    st, r = api('POST', '/api/v1/activity/cache-warmup', admin_token, {'product_id': pid})
    check('Redis库存预热', st == 200, f'msg={r.get("message")}')

    # ---------- 3. 安全校验（普通用户必须验证码/路径Token） ----------
    print('\n[3] 验证码/路径Token 强制校验')
    pt, cid, ans = fresh_path_captcha(tA, pid)
    st, r = do_seckill(tA, pid, pt, cid, 999, 'it-wrong-' + str(int(time.time())))
    check('错误验证码被拒(400)', st == 200 and r.get('code') == 400, f'msg={r.get("message")}')

    pt, cid, ans = fresh_path_captcha(tA, pid)
    st, r = api('POST', '/api/v1/seckill', tA, {
        'product_id': pid, 'quantity': 1, 'idempotent_key': 'it-nocap-' + str(int(time.time())),
        'path_token': pt})
    check('缺失验证码被拒(400)', st == 200 and r.get('code') == 400, f'msg={r.get("message")}')

    # 路径Token一次性：用过的 Token 再次提交必须失败
    pt, cid, ans = fresh_path_captcha(tA, pid)
    st, r = do_seckill(tA, pid, pt, cid, ans, 'it-pt-' + str(int(time.time())))
    first_ok = (st == 200 and r.get('code') == 200)
    st2, r2 = do_seckill(tA, pid, pt, cid, ans, 'it-pt2-' + str(int(time.time())))
    check('路径Token一次性消费', first_ok and st2 == 200 and r2.get('code') == 400,
          f'first={r.get("code")} replay={r2.get("code")} msg={r2.get("message")}')
    time.sleep(0.3)

    # ---------- 4. 限购语义（每用户2件） ----------
    print('\n[4] 限购校验（limit=2）')
    pt, cid, ans = fresh_path_captcha(tA, pid)
    st, r = do_seckill(tA, pid, pt, cid, ans, 'it-limit-2-' + str(int(time.time())))
    check('第2件购买成功(达上限)', st == 200 and r.get('code') == 200, f'msg={r.get("message")}')
    pt, cid, ans = fresh_path_captcha(tA, pid)
    st, r = do_seckill(tA, pid, pt, cid, ans, 'it-limit-3-' + str(int(time.time())))
    check('第3件限购拒绝(400)', st == 200 and r.get('code') == 400 and '限购' in r.get('message', ''),
          f'msg={r.get("message")}')
    time.sleep(0.3)

    # ---------- 5. 幂等性（独立用户B，避免受限购干扰） ----------
    print('\n[5] 幂等性校验（用户B）')
    pt, cid, ans = fresh_path_captcha(tB, pid)
    idem = 'it-idem-' + str(int(time.time()))
    st, r = do_seckill(tB, pid, pt, cid, ans, idem)
    first_ok = (st == 200 and r.get('code') == 200)
    pt, cid, ans = fresh_path_captcha(tB, pid)
    st2, r2 = do_seckill(tB, pid, pt, cid, ans, idem)
    check('幂等Key重复提交被拒(400)', first_ok and st2 == 200 and r2.get('code') == 400,
          f'first={r.get("code")} replay={r2.get("code")} msg={r2.get("message")}')
    time.sleep(0.3)

    # ---------- 6. 管理员秒杀（豁免验证码）+ 超卖防护 ----------
    print('\n[6] 管理员秒杀 + 超卖防护（库存=5，已耗4）')
    # 已消耗: A2单 + B1单 = 3 单，剩 2；admin 限购=2 也适用
    st, r = api('POST', '/api/v1/seckill', admin_token,
                {'product_id': pid, 'quantity': 1, 'idempotent_key': 'it-admin-1-' + str(int(time.time()))})
    check('管理员秒杀(无验证码)成功', st == 200 and r.get('code') == 200, f'msg={r.get("message")}')
    st, r = api('POST', '/api/v1/seckill', admin_token,
                {'product_id': pid, 'quantity': 1, 'idempotent_key': 'it-admin-2-' + str(int(time.time()))})
    check('管理员第2单成功(库存清零)', st == 200 and r.get('code') == 200, f'msg={r.get("message")}')
    st, r = api('POST', '/api/v1/seckill', admin_token,
                {'product_id': pid, 'quantity': 1, 'idempotent_key': 'it-admin-3-' + str(int(time.time()))})
    check('超限/超卖被拒(400) 无超卖', st == 200 and r.get('code') == 400, f'msg={r.get("message")}')

    # ---------- 7. 订单生命周期：支付/取消回滚 ----------
    print('\n[7] 订单生命周期（支付回调 + 取消回滚）')
    _, r = api('GET', '/api/v1/order/list?page=1&page_size=20', tB)
    b_orders = r.get('data', {}).get('list', [])
    b1 = next((o for o in b_orders if o.get('product_id') == pid), None)
    check('用户B订单可查询', b1 is not None)
    if b1:
        st, r = api('POST', '/api/v1/order/pay-callback', tB, {'order_no': b1['order_no'], 'pay_sign': 'M2-mock-sign'})
        check('支付回调成功', st == 200 and r.get('code') == 200, f'msg={r.get("message")}')
        _, r = api('GET', f"/api/v1/order/{b1['order_no']}", tB)
        check('订单状态=已支付(1)', r.get('data', {}).get('status') == 1, f'status={r.get("data", {}).get("status")}')

    # 用户A取消待支付订单 A1 → 库存回滚 + 限购计数递减
    _, r = api('GET', '/api/v1/order/list?page=1&page_size=20', tA)
    a_orders = r.get('data', {}).get('list', [])
    a1 = next((o for o in a_orders if o.get('product_id') == pid and o.get('status') == 0), None)
    check('用户A有待支付订单可取消', a1 is not None)
    if a1:
        st, r = api('POST', '/api/v1/order/cancel', tA, {'order_no': a1['order_no']})
        check('待支付订单取消成功', st == 200 and r.get('code') == 200, f'msg={r.get("message")}')
        _, r = api('GET', f"/api/v1/order/{a1['order_no']}", tA)
        check('订单状态=已取消(2)', r.get('data', {}).get('status') == 2, f'status={r.get("data", {}).get("status")}')
        # 取消后库存回滚 + 限购计数递减 → 用户A可再买1件（原计数2→1，现再+1=2）
        pt, cid, ans = fresh_path_captcha(tA, pid)
        st, r = do_seckill(tA, pid, pt, cid, ans, 'it-rollback-' + str(int(time.time())))
        check('取消回滚后可再秒杀1件', st == 200 and r.get('code') == 200, f'msg={r.get("message")}')

    # ---------- 8. 最终一致性核对 ----------
    print('\n[8] 最终一致性核对')
    _, r = api('GET', f'/api/v1/product/{pid}', admin_token)
    remain = r.get('data', {}).get('remain_stock', -1)
    check('商品剩余库存=0（无超卖、回滚守恒）', remain == 0, f'remain={remain}')

    print('\n' + '=' * 60)
    print(f' 集成测试结果: PASS={PASS}  FAIL={FAIL}')
    print('=' * 60)
    sys.exit(1 if FAIL else 0)


if __name__ == '__main__':
    main()
