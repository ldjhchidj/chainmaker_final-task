#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""基于长安链的学历证书存证系统 —— 链下 Web 应用层

技术栈：Flask 3.1 + SQLite（用户/角色） + bcrypt（口令散列） + ChainMaker CMC（链交互）

运行前请确认以下 3 个环境变量（或直接修改下方的默认值）指向你的虚拟机实际路径：
    CERT_CMC_PATH   cmc 可执行文件路径，默认 ~/chainmaker-lab/chainmaker-go/tools/cmc/cmc
    CERT_SDK_CONF   sdk_config.yml 路径，默认 <cmc 目录>/testdata/sdk_config.yml
    CERT_CONTRACT   合约名称，默认 Web777

启动： python3 app.py   →   浏览器访问 http://<虚拟机IP>:5000
"""

import base64
import json
import os
import re
import sqlite3
import subprocess

import bcrypt
from flask import Flask, g, redirect, render_template, request, session, url_for

# ---------------------------------------------------------------- 配置

BASE_DIR = os.path.dirname(os.path.abspath(__file__))

CMC_PATH = os.environ.get(
    "CERT_CMC_PATH",
    os.path.expanduser("~/chainmaker-lab/chainmaker-go/tools/cmc/cmc"),
)
SDK_CONF_PATH = os.environ.get(
    "CERT_SDK_CONF",
    os.path.expanduser("~/chainmaker-lab/chainmaker-go/tools/cmc/testdata/sdk_config.yml"),
)
CONTRACT_NAME = os.environ.get("CERT_CONTRACT", "Web777")
DB_PATH = os.environ.get("CERT_DB", os.path.join(BASE_DIR, "cert_system.db"))
CMC_TIMEOUT = int(os.environ.get("CERT_CMC_TIMEOUT", "30"))

# 角色说明（与合约注释中的权限设计一致）
ROLE_LABELS = {
    "query": "查询机构",
    "uni": "高校",
    "edu": "教育部",
}

# 内置账号（首次运行写入 SQLite；口令以 bcrypt 散列存储）
DEFAULT_USERS = {
    "query_org": ("Query@2024", "query"),
    "uni_org_bupt": ("Uni@2024", "uni"),
    "edu_org_moe": ("Edu@2024", "edu"),
}

app = Flask(__name__)
app.secret_key = os.environ.get("CERT_SECRET_KEY", "chainmaker_cert_system_secure_key")

# ---------------------------------------------------------------- SQLite


def get_db():
    if "_database" not in g:
        g._database = sqlite3.connect(DB_PATH, check_same_thread=False)
    return g._database


@app.teardown_appcontext
def close_db(_exc):
    db = g.pop("_database", None)
    if db is not None:
        db.close()


def init_db():
    db = sqlite3.connect(DB_PATH)
    try:
        db.execute(
            """
            CREATE TABLE IF NOT EXISTS users (
                id       INTEGER PRIMARY KEY AUTOINCREMENT,
                username TEXT UNIQUE NOT NULL,
                password TEXT NOT NULL,
                role     TEXT NOT NULL          -- query / uni / edu
            )
            """
        )
        db.commit()
    finally:
        db.close()


def init_users():
    db = sqlite3.connect(DB_PATH)
    try:
        for username, (password, role) in DEFAULT_USERS.items():
            row = db.execute("SELECT id FROM users WHERE username = ?", (username,)).fetchone()
            if row:
                continue
            hashed = bcrypt.hashpw(password.encode("utf-8"), bcrypt.gensalt())
            db.execute(
                "INSERT INTO users (username, password, role) VALUES (?, ?, ?)",
                (username, hashed, role),
            )
        db.commit()
    finally:
        db.close()


# ---------------------------------------------------------------- 模板过滤器


@app.template_filter("b64decode")
def b64decode_filter(value):
    """Base64 解码（兼容空值与非法输入）。"""
    if not value:
        return ""
    try:
        return base64.b64decode(value).decode("utf-8")
    except Exception:  # noqa: BLE001 - 解码失败时返回空串，由页面提示
        return ""


@app.template_filter("fromjson")
def fromjson_filter(value):
    """JSON 反序列化（失败时返回空对象）。"""
    if not value:
        return None
    try:
        return json.loads(value)
    except json.JSONDecodeError:
        return None


@app.template_filter("role_label")
def role_label_filter(role):
    return ROLE_LABELS.get(role, role or "")


# ---------------------------------------------------------------- 链交互封装


def _extract_json(text):
    """从 cmc 输出中提取 JSON（容忍前置日志行）。"""
    text = (text or "").strip()
    if not text:
        return None
    try:
        return json.loads(text)
    except json.JSONDecodeError:
        pass
    match = re.search(r"\{.*\}", text, re.S)
    if match:
        try:
            return json.loads(match.group(0))
        except json.JSONDecodeError:
            return None
    return None


def _parse_contract_response(data):
    """把 cmc 返回的 JSON 归一化为前端可直接渲染的结构。"""
    contract_result = data.get("contract_result") or {}
    top_code = data.get("code", 0)
    code = contract_result.get("code", top_code)
    message = contract_result.get("message", data.get("message", ""))
    raw_b64 = contract_result.get("result", data.get("result", "")) or ""

    text = b64decode_filter(raw_b64)
    payload = fromjson_filter(text)

    return {
        "ok": code in (0, "0") and top_code in (0, "0"),
        "code": code,
        "message": message or "",
        "tx_id": data.get("tx_id", ""),
        "b64": raw_b64,
        "text": text,
        "payload": payload,
    }


def call_cmc(method, params, write=True):
    """调用合约。

    write=True  → cmc client contract user invoke（上链交易，产生交易 ID，可留审计日志）
    write=False → cmc client contract user get（链上查询，不产生交易）
    """
    sub_cmd = "invoke" if write else "get"
    cmd = [
        CMC_PATH,
        "client",
        "contract",
        "user",
        sub_cmd,
        f"--contract-name={CONTRACT_NAME}",
        f"--method={method}",
        f"--sdk-conf-path={SDK_CONF_PATH}",
        f"--params={json.dumps(params, ensure_ascii=False)}",
    ]
    if write:
        cmd.append("--sync-result=true")

    try:
        proc = subprocess.run(cmd, capture_output=True, text=True, timeout=CMC_TIMEOUT)
    except FileNotFoundError:
        return {"error": f"找不到 cmc 可执行文件：{CMC_PATH}，请检查 CERT_CMC_PATH 配置"}
    except subprocess.TimeoutExpired:
        return {"error": f"cmc 命令执行超时（{CMC_TIMEOUT} 秒）"}
    except Exception as exc:  # noqa: BLE001
        return {"error": f"调用 cmc 过程出错：{exc}"}

    if proc.returncode != 0:
        detail = (proc.stderr or proc.stdout or "").strip()
        return {"error": f"cmc 执行失败（退出码 {proc.returncode}）：{detail[:400]}"}

    data = _extract_json(proc.stdout)
    if data is None:
        return {"error": "解析 cmc 输出失败，原始输出：" + (proc.stdout or "")[:400]}

    return _parse_contract_response(data)


# ---------------------------------------------------------------- 权限控制


def current_user():
    return {"username": session.get("username", ""), "role": session.get("role", "")}


def require_login():
    return "username" not in session


def forbid(role, message):
    """角色不符时返回 403 页面。"""
    return render_template("403.html", message=message, role=role), 403


# ---------------------------------------------------------------- 路由：登录 / 首页


@app.route("/login", methods=["GET", "POST"])
def login():
    if request.method == "POST":
        username = request.form.get("username", "").strip()
        password = request.form.get("password", "").strip()
        if not username or not password:
            return render_template("login.html", error="用户名和密码不能为空")

        row = (
            get_db()
            .execute("SELECT username, password, role FROM users WHERE username = ?", (username,))
            .fetchone()
        )
        if row is None:
            return render_template("login.html", error="账号或密码错误")

        stored = row[1]
        if isinstance(stored, str):
            stored = stored.encode("utf-8")
        try:
            if not bcrypt.checkpw(password.encode("utf-8"), stored):
                return render_template("login.html", error="账号或密码错误")
        except ValueError:
            return render_template("login.html", error="登录过程出错，请联系管理员")

        session["username"] = row[0]
        session["role"] = row[2]
        return redirect(url_for("index"))

    return render_template("login.html", error=None)


@app.route("/logout")
def logout():
    session.clear()
    return redirect(url_for("login"))


@app.route("/")
def index():
    if require_login():
        return redirect(url_for("login"))
    return render_template("index.html", user=current_user())


@app.route("/profile")
def profile():
    if require_login():
        return redirect(url_for("login"))
    return render_template("profile.html", user=current_user())


# ---------------------------------------------------------------- 路由：查询（全部角色）


@app.route("/query", methods=["GET", "POST"])
def query_cert():
    if require_login():
        return redirect(url_for("login"))
    user = current_user()

    if request.method == "GET":
        return render_template("query.html", user=user, result=None, form={})

    form = {
        "student_id": request.form.get("student_id", "").strip(),
        "name": request.form.get("name", "").strip(),
        "cert_id": request.form.get("cert_id", "").strip(),
    }
    has_cert_id = bool(form["cert_id"])
    has_student = bool(form["student_id"]) or bool(form["name"])

    if has_cert_id and has_student:
        return render_template(
            "query.html",
            user=user,
            form=form,
            result={"error": "证书 ID 与学生 ID + 姓名不能同时填写，请二选一查询"},
        )
    if not has_cert_id and not has_student:
        return render_template(
            "query.html",
            user=user,
            form=form,
            result={"error": "请填写证书 ID，或学生 ID + 姓名"},
        )
    if has_student and not (form["student_id"] and form["name"]):
        return render_template(
            "query.html",
            user=user,
            form=form,
            result={"error": "按学生信息查询时，学生 ID 与姓名必须同时填写"},
        )

    # 查询也走上链交易（invoke），使合约能写入 QUERY 审计日志
    result = call_cmc("queryCertificate", form, write=True)
    return render_template("query.html", user=user, form=form, result=result)


@app.route("/verify", methods=["GET", "POST"])
def verify_cert():
    if require_login():
        return redirect(url_for("login"))
    user = current_user()
    cert_id = request.form.get("cert_id", "").strip()

    if request.method == "GET":
        return render_template("verify.html", user=user, result=None, cert_id="")
    if not cert_id:
        return render_template(
            "verify.html", user=user, result={"error": "证书 ID 不能为空"}, cert_id=cert_id
        )

    # 校验是只读操作，使用 get，不产生交易
    result = call_cmc("verifyCertificate", {"cert_id": cert_id}, write=False)
    return render_template("verify.html", user=user, result=result, cert_id=cert_id)


# ---------------------------------------------------------------- 路由：颁发（高校）


@app.route("/issue", methods=["GET", "POST"])
def issue_cert():
    if require_login():
        return redirect(url_for("login"))
    user = current_user()
    if user["role"] != "uni":
        return forbid(user["role"], "颁发证书仅限高校账号（uni）操作")

    if request.method == "GET":
        return render_template("issue.html", user=user, result=None, form={})

    form = {
        "student_id": request.form.get("student_id", "").strip(),
        "name": request.form.get("name", "").strip(),
        "eduction": request.form.get("eduction", "").strip(),
        "school_name": request.form.get("school_name", "").strip(),
        "major": request.form.get("major", "").strip(),
        "issue_date": request.form.get("issue_date", "").strip(),
        "graduation_date": request.form.get("graduation_date", "").strip(),
    }
    required = ["student_id", "name", "eduction", "school_name", "major", "issue_date", "graduation_date"]
    missing = [key for key in required if not form[key]]
    if missing:
        return render_template(
            "issue.html",
            user=user,
            form=form,
            result={"error": "缺少必填字段：" + "、".join(missing)},
        )
    for key in ("issue_date", "graduation_date"):
        if not re.fullmatch(r"\d{8}", form[key]):
            return render_template(
                "issue.html",
                user=user,
                form=form,
                result={"error": f"{key} 需为 YYYYMMDD 格式的 8 位数字"},
            )

    result = call_cmc("issueCertificate", form, write=True)
    return render_template("issue.html", user=user, form=form, result=result)


# ---------------------------------------------------------------- 路由：撤销（教育部）


@app.route("/revoke", methods=["GET", "POST"])
def revoke_cert():
    if require_login():
        return redirect(url_for("login"))
    user = current_user()
    if user["role"] != "edu":
        return forbid(user["role"], "撤销证书仅限教育部账号（edu）操作")

    if request.method == "GET":
        return render_template("revoke.html", user=user, result=None, form={})

    form = {
        "cert_id": request.form.get("cert_id", "").strip(),
        "revoke_reason": request.form.get("revoke_reason", "").strip(),
        "operator_id": user["username"],
    }
    if not form["cert_id"]:
        return render_template("revoke.html", user=user, form=form, result={"error": "证书 ID 不能为空"})
    if not form["revoke_reason"]:
        return render_template("revoke.html", user=user, form=form, result={"error": "撤销原因不能为空"})

    result = call_cmc("revokeCertificate", form, write=True)
    return render_template("revoke.html", user=user, form=form, result=result)


# ---------------------------------------------------------------- 路由：全量台账（教育部）


@app.route("/list-all")
def list_all_certs():
    if require_login():
        return redirect(url_for("login"))
    user = current_user()
    if user["role"] != "edu":
        return forbid(user["role"], "全量证书台账仅限教育部账号（edu）查看")

    result = call_cmc("listAllCertificates", {}, write=False)
    certs = result.get("payload") if result.get("ok") else None
    if not isinstance(certs, list):
        certs = []
    return render_template("list_all.html", user=user, result=result, certs=certs)


# ---------------------------------------------------------------- 错误页


@app.errorhandler(404)
def page_not_found(_e):
    return render_template("404.html"), 404


@app.errorhandler(500)
def internal_error(_e):
    return render_template("500.html"), 500


# ---------------------------------------------------------------- 启动


def bootstrap():
    init_db()
    init_users()


bootstrap()

if __name__ == "__main__":
    print("=" * 62)
    print("学历证书存证系统  |  ChainMaker + Flask")
    print(f"  cmc        : {CMC_PATH}")
    print(f"  sdk 配置    : {SDK_CONF_PATH}")
    print(f"  合约名称    : {CONTRACT_NAME}")
    print(f"  数据库      : {DB_PATH}")
    print("  访问地址    : http://0.0.0.0:5000")
    print("=" * 62)
    app.run(host="0.0.0.0", port=int(os.environ.get("CERT_PORT", "5000")), debug=False)
