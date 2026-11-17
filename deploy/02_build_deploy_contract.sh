#!/usr/bin/env bash
# 任务二 + 任务三：编译 cmc、准备 testdata、部署证书合约、跑通 4 个核心方法
#
# 用法：
#   bash deploy/02_build_deploy_contract.sh                # 只部署 + 冒烟测试
#   SKIP_CREATE=1 bash deploy/02_build_deploy_contract.sh  # 合约已部署，只做调用测试
#
# 需要的合约字节码：由 SmartIDE 编译 CertContract（contracts/CertContract/main.go）后下载的
# cert-contract-v2.7z 之类文件，放在 $CMC_DIR/testdata/claim-docker-go-demo/ 下，
# 或用环境变量 CONTRACT_BYTECODE 指定绝对路径。
set -euo pipefail

WORKDIR="${WORKDIR:-$HOME/chainmaker-lab}"
CHAIN_DIR="$WORKDIR/chainmaker-go"
CMC_DIR="$CHAIN_DIR/tools/cmc"
CONTRACT_NAME="${CONTRACT_NAME:-Web777}"
CONTRACT_BYTECODE="${CONTRACT_BYTECODE:-./testdata/claim-docker-go-demo/cert-contract-v2.7z}"

cd "$CMC_DIR"

echo "==> 1) 编译 cmc（已存在则跳过）"
if [ ! -x ./cmc ]; then
  go build
fi
./cmc --help > /dev/null && echo "    cmc 可用：$(pwd)/cmc"

echo "==> 2) 准备 testdata/crypto-config（已存在则跳过）"
if [ ! -d ./testdata/crypto-config ]; then
  cp -rf "$CHAIN_DIR/build/crypto-config" ./testdata/
fi
ls ./testdata/crypto-config | head -n 4

echo "==> 3) 检查 sdk_config.yml 是否为绝对路径"
CONF=./testdata/sdk_config.yml
if [ ! -f "$CONF" ]; then
  echo "!! 缺少 $CONF，请从 deploy/sdk_config.template.yml 复制并改成绝对路径" >&2
  exit 1
fi
if grep -q '"\./' "$CONF"; then
  echo "!! $CONF 中仍有 \"./\" 相对路径，指导手册要求改为绝对路径（如 $CMC_DIR/testdata/...）" >&2
  exit 1
fi
echo "    sdk_config.yml 路径检查通过"

echo "==> 4) 连通性自检"
./cmc client chainconfig block height --sdk-conf-path="$CONF" | head -n 5

if [ "${SKIP_CREATE:-0}" != "1" ]; then
  echo "==> 5) 创建合约 $CONTRACT_NAME（DOCKER_GO）"
  if [ ! -f "$CONTRACT_BYTECODE" ]; then
    echo "!! 未找到合约字节码 $CONTRACT_BYTECODE" >&2
    echo "   请到 SmartIDE 编译 contracts/CertContract/main.go，下载 7z 后放到 $CMC_DIR/testdata/claim-docker-go-demo/" >&2
    exit 1
  fi
  ./cmc client contract user create \
    --contract-name="$CONTRACT_NAME" \
    --runtime-type=DOCKER_GO \
    --byte-code-path="$CONTRACT_BYTECODE" \
    --version=1.0 \
    --sdk-conf-path="$CONF" \
    --admin-key-file-paths=./testdata/crypto-config/wx-org1.chainmaker.org/user/admin1/admin1.tls.key,./testdata/crypto-config/wx-org2.chainmaker.org/user/admin1/admin1.tls.key,./testdata/crypto-config/wx-org3.chainmaker.org/user/admin1/admin1.tls.key,./testdata/crypto-config/wx-org4.chainmaker.org/user/admin1/admin1.tls.key \
    --admin-crt-file-paths=./testdata/crypto-config/wx-org1.chainmaker.org/user/admin1/admin1.tls.crt,./testdata/crypto-config/wx-org2.chainmaker.org/user/admin1/admin1.tls.crt,./testdata/crypto-config/wx-org3.chainmaker.org/user/admin1/admin1.tls.crt,./testdata/crypto-config/wx-org4.chainmaker.org/user/admin1/admin1.tls.crt \
    --sync-result=true \
    --params="{}"
fi

echo "==> 6) 冒烟测试：颁发 → 查询 → 全量 → 验真 → 撤销"
./cmc client contract user invoke \
  --contract-name="$CONTRACT_NAME" \
  --method=issueCertificate \
  --sdk-conf-path="$CONF" \
  --params='{"student_id":"2025211762","name":"杨秩宇","eduction":"本科","school_name":"北京邮电大学","major":"计算机科学与技术","issue_date":"20260701","graduation_date":"20260620"}' \
  --sync-result=true

echo "--- 查询（返回 base64，可用 base64 -d 查看明文）"
./cmc client contract user get \
  --contract-name="$CONTRACT_NAME" \
  --method=queryCertificate \
  --sdk-conf-path="$CONF" \
  --params='{"student_id":"2025211762","name":"杨秩宇"}'

echo "--- 全量台账"
./cmc client contract user get \
  --contract-name="$CONTRACT_NAME" \
  --method=listAllCertificates \
  --sdk-conf-path="$CONF" \
  --params='{}'

echo
echo "部署与冒烟测试完成。撤销与验真命令见 docs/03-合约与接口说明.md"
echo "下一步： bash deploy/03_start_web.sh"
