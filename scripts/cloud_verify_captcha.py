# -*- coding: utf-8 -*-
"""
云端验证（2026-09-05 方案2）：admin 强制验证码校验
流程：登录 admin → 取商品 → 缺失验证码被拒(400) → 错误验证码被拒(400) → 正确验证码成功(200)
注意：云端 API 必须带 HMAC 签名头（X-Timestamp/X-Sign），login 为签名白名单
"""
import json, time, hmac, hashlib, urllib.request, urllib.error

BASE = 'http://115.159.157.18'
SECRET = 'miaosha-sign-secret-2026'
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
        with urllib.request.urlopen(req, timeout=20) as r:
            return r.status, json.loads(r.read().decode())
    except urllib.error.HTTPError as e:
        try:
            return e.code, json.loads(e.read().decode())
        except Exception:
            return e.code, {}


def solve(expr):
    for op in ('+', '-'):
        if op in expr:
            a, b = expr.split(op)
            return int(a.strip()) + int(b.strip()) if op == '+' else int(a.strip()) - int(b.strip())
    raise ValueError(f'无法解析: {expr}')


def main():
    print('=' * 60)
    print(' 云端验证 — 方案2: admin 强制验证码（115.159.157.18）')
    print('=' * 60)

    # 1. admin 登录（白名单）
    st, r = api('POST', '/api/v1/user/login', body={'username': 'admin', 'password': 'test123'})
    token = r.get('data', {}).get('token', '')
    check('admin 登录', st == 200 and token, f'st={st}')

    # 2. 秒杀商品列表（带签名）
    st, r = api('GET', '/api/v1/product/list', token)
    products = (r.get('data') or {}).get('list') or []
    pid = None
    for p in products:
        if p.get('status') == 1:
            pid = p.get('id')
            break
    check('获取秒杀商品', pid is not None, f'products={len(products)}')
    if not pid:
        print('! 无可用商品，终止'); return

    # 3. 方案2 核心断言：admin 缺失验证码 → 400
    st, r = api('POST', '/api/v1/seckill', token,
                {'product_id': pid, 'quantity': 1, 'idempotent_key': f'cv-no-captcha-{int(time.time())}'})
    check('admin 缺失验证码被拒(400)', st == 200 and r.get('code') == 400,
          f'code={r.get("code")} msg={r.get("message")}')

    # 4. admin 错误验证码 → 400
    st, cp = api('GET', f'/api/v1/seckill/captcha?product_id={pid}', token)
    cid = cp['data']['captcha_id']
    wrong = (solve(cp['data']['expression']) + 1) % 20
    st, r = api('POST', '/api/v1/seckill', token,
                {'product_id': pid, 'quantity': 1, 'idempotent_key': f'cv-wrong-{int(time.time())}',
                 'captcha_id': cid, 'captcha_code': wrong})
    check('admin 错误验证码被拒(400)', st == 200 and r.get('code') == 400,
          f'code={r.get("code")} msg={r.get("message")}')

    # 5. admin 正确验证码 → 200（admin 仍豁免 path_token）
    st, cp = api('GET', f'/api/v1/seckill/captcha?product_id={pid}', token)
    cid = cp['data']['captcha_id']
    ans = solve(cp['data']['expression'])
    st, r = api('POST', '/api/v1/seckill', token,
                {'product_id': pid, 'quantity': 1, 'idempotent_key': f'cv-ok-{int(time.time())}',
                 'captcha_id': cid, 'captcha_code': ans})
    check('admin 正确验证码秒杀成功(200)', st == 200 and r.get('code') == 200,
          f'code={r.get("code")} msg={r.get("message")} order={r.get("data", {}).get("order_no")}')

    print(f'\n结果: PASS={PASS} FAIL={FAIL}')
    sys_exit = 1 if FAIL else 0
    import sys
    sys.exit(sys_exit)


if __name__ == '__main__':
    main()
