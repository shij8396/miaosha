# -*- coding: utf-8 -*-
"""实测云端验证码漏洞：错误答案是否被拦截"""
import sys
import json
import time
import hmac
import hashlib
import urllib.request

if sys.platform == 'win32':
    sys.stdout.reconfigure(encoding='utf-8', errors='replace')

BASE = 'http://115.159.157.18'
SECRET = 'miaosha-sign-secret-2026'


def api(method, path, token=None, body=None):
    url = BASE + path
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(url, data=data, method=method)
    req.add_header('Content-Type', 'application/json')
    if token:
        req.add_header('Authorization', 'Bearer ' + token)
    # 复刻前端签名: HMAC-SHA256(ts + METHOD + path + body)
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


# 1. 登录 testuser
st, r = api('POST', '/api/v1/user/login', body={'username': 'testuser', 'password': 'test123'})
token = r['data']['token']
print('登录:', st, r.get('message'))

# 2. 获取验证码（故意不看答案）
try:
    st, cap = api('GET', '/api/v1/seckill/captcha?product_id=3', token)
    print('captcha响应:', st, json.dumps(cap, ensure_ascii=False)[:300])
    cap = cap['data']
except Exception as ex:
    print('captcha调用异常:', repr(ex))
    raise
print('验证码:', cap['expression'], 'id=', cap['captcha_id'][:12], '(不计算答案)')

# 3. 获取 path token
st, pt = api('GET', '/api/v1/seckill/path?product_id=3', token)
path_token = pt['data']['path_token']
print('pathToken:', st, path_token[:12] if path_token else pt)

# 4. 提交错误答案 (999)
st, r = api('POST', '/api/v1/seckill', token, {
    'product_id': 3, 'quantity': 1, 'idempotent_key': 'test-wrong-cap-' + str(int(time.time())),
    'path_token': path_token, 'captcha_code': 999, 'captcha_id': cap['captcha_id']
})
print('错误答案提交 => HTTP', st, json.dumps(r, ensure_ascii=False))

# 5. 再测：完全不传验证码字段（模拟脚本绕过）
st2, pt2 = api('GET', '/api/v1/seckill/path?product_id=3', token)
st, r = api('POST', '/api/v1/seckill', token, {
    'product_id': 3, 'quantity': 1, 'idempotent_key': 'test-nocap-' + str(int(time.time())),
    'path_token': pt2['data']['path_token']
})
print('不传验证码提交 => HTTP', st, json.dumps(r, ensure_ascii=False))
