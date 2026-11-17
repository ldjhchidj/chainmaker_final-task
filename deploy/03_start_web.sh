#!/usr/bin/env bash
# 任务四 + 任务五：启动 Flask 图形化界面
#
# 用法：
#   bash deploy/03_start_web.sh
#   CERT_PORT=8080 bash deploy/03_start_web.sh
#
# 若 cmc / sdk_config.yml 不在默认路径，先导出环境变量：
#   export CERT_CMC_PATH=/home/<user>/chainmaker-lab/chainmaker-go/tools/cmc/cmc
#   export CERT_SDK_CONF=/home/<user>/chainmaker-lab/chainmaker-go/tools/cmc/testdata/sdk_config.yml
#   export CERT_CONTRACT=Web777
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WEB_DIR="$REPO_DIR/web"
PYTHON="${PYTHON:-python3}"

cd "$WEB_DIR"

echo "==> 1) 安装 Python 依赖"
"$PYTHON" -m pip install --quiet --upgrade pip
"$PYTHON" -m pip install --quiet -r requirements.txt
"$PYTHON" - <<'PY'
import flask, bcrypt
print(f"    Flask {flask.__version__} / bcrypt {bcrypt.__version__}")
PY

echo "==> 2) 启动 Web 服务（Ctrl+C 停止）"
echo "    cmc    : ${CERT_CMC_PATH:-~/chainmaker-lab/chainmaker-go/tools/cmc/cmc}"
echo "    sdk    : ${CERT_SDK_CONF:-~/chainmaker-lab/chainmaker-go/tools/cmc/testdata/sdk_config.yml}"
echo "    合约   : ${CERT_CONTRACT:-Web777}"
echo "    访问   : http://$(hostname -I | awk '{print $1}'):${CERT_PORT:-5000}"
exec "$PYTHON" app.py
