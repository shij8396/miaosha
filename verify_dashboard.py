# -*- coding: utf-8 -*-
"""数据大屏增强接口本地验证脚本：登录 -> 依次调用新监控接口并打印结果"""
import json, time, hmac, hashlib, sys
import requests

BASE = "http://127.0.0.1:8080"
SECRET = "miaosha-sign-secret-2026"

def sign_headers(method, path, body=""):
    ts = str(int(time.time()))
    payload = ts + method + path + (body or "")
    s = hmac.new(SECRET.encode(), payload.encode(), hashlib.sha256).hexdigest()
    return {"X-Timestamp": ts, "X-Sign": s, "Content-Type": "application/json"}

def call(method, path, token=None, body=None):
    body_str = json.dumps(body) if body else ""
    h = sign_headers(method, path.split("?")[0], body_str)
    if token:
        h["Authorization"] = "Bearer " + token
    r = requests.request(method, BASE + path, headers=h, data=body_str, timeout=10)
    return r.status_code, r.json()

# 1. 登录（本地库无 testuser 时注册临时账号验证）
token = None
for u, p in [("testuser", "test123"), ("admin", "admin123"), ("admin", "test123")]:
    code, resp = call("POST", "/api/v1/user/login", body={"username": u, "password": p})
    if (resp.get("data") or {}).get("token"):
        token = resp["data"]["token"]
        print(f"LOGIN OK: {u}")
        break
if not token:
    ts = str(int(time.time()))
    body = {"username": "verify_dash", "password": "Verify123456", "email": "vd@t.cn"}
    code, resp = call("POST", "/api/v1/user/register", body=body)
    print("REGISTER:", code, resp.get("code"), resp.get("message", ""))
    code, resp = call("POST", "/api/v1/user/login", body={"username": "verify_dash", "password": "Verify123456"})
    token = (resp.get("data") or {}).get("token")
if not token:
    print("!! 登录失败，无法继续验证"); sys.exit(1)

# 2. 依次调用大屏接口
for name, path in [
    ("秒杀统计(含PV/UV/转化率)", "/api/v1/seckill/stats"),
    ("实时流量PV/UV", "/api/v1/monitor/pvuv"),
    ("热销商品排行", "/api/v1/monitor/hot-products?top=5"),
    ("库存状态", "/api/v1/monitor/inventory"),
    ("中间件状态", "/api/v1/monitor/middleware"),
    ("告警列表", "/api/v1/monitor/alarms"),
    ("QPS历史", "/api/v1/monitor/qps"),
]:
    code, resp = call("GET", path, token=token)
    data = resp.get("data")
    brief = json.dumps(data, ensure_ascii=False)[:300]
    print(f"\n=== {name} [{code}/{resp.get('code')}] ===")
    print(brief)
