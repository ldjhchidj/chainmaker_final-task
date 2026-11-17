#!/usr/bin/env bash
# 合约方法手工验证清单（对应报告截图用）
#
# 用法：
#   export CERT_ID=cert_2025211762_xxxxxxxx     # 上一步颁发的证书 ID
#   bash deploy/test_contract.sh query-student
#   bash deploy/test_contract.sh query-cert
#   bash deploy/test_contract.sh list
#   bash deploy/test_contract.sh verify
#   bash deploy/test_contract.sh revoke
#
# 说明：query / list / verify 使用 get（不打交易），
#       issue / revoke 使用 invoke（同步出块，返回交易 ID 与区块高度）。
set -euo pipefail

WORKDIR="${WORKDIR:-$HOME/chainmaker-lab}"
CMC_DIR="$WORKDIR/chainmaker-go/tools/cmc"
CONTRACT_NAME="${CONTRACT_NAME:-Web777}"
CONF="./testdata/sdk_config.yml"
CERT_ID="${CERT_ID:-}"
STUDENT_ID="${STUDENT_ID:-2025211762}"
STUDENT_NAME="${STUDENT_NAME:-杨秩宇}"

cd "$CMC_DIR"

need_cert_id() {
  if [ -z "$CERT_ID" ]; then
    echo "!! 请先导出证书 ID： export CERT_ID=cert_xxx_xxxxxxxx" >&2
    exit 1
  fi
}

decode() { base64 -d 2>/dev/null | python3 -m json.tool 2>/dev/null || echo "(非 JSON 返回)"; }

case "${1:-help}" in
  query-student)
    echo "### 按 学生ID + 姓名 查询"
    ./cmc client contract user get \
      --contract-name="$CONTRACT_NAME" \
      --method=queryCertificate \
      --sdk-conf-path="$CONF" \
      --params="{\"student_id\":\"$STUDENT_ID\",\"name\":\"$STUDENT_NAME\"}"
    ;;

  query-cert)
    need_cert_id
    echo "### 按 证书ID 查询"
    ./cmc client contract user get \
      --contract-name="$CONTRACT_NAME" \
      --method=queryCertificate \
      --sdk-conf-path="$CONF" \
      --params="{\"cert_id\":\"$CERT_ID\"}"
    ;;

  list)
    echo "### 全量证书台账"
    ./cmc client contract user get \
      --contract-name="$CONTRACT_NAME" \
      --method=listAllCertificates \
      --sdk-conf-path="$CONF" \
      --params='{}'
    ;;

  verify)
    need_cert_id
    echo "### 哈希验真"
    ./cmc client contract user get \
      --contract-name="$CONTRACT_NAME" \
      --method=verifyCertificate \
      --sdk-conf-path="$CONF" \
      --params="{\"cert_id\":\"$CERT_ID\"}"
    ;;

  revoke)
    need_cert_id
    echo "### 撤销证书"
    ./cmc client contract user invoke \
      --contract-name="$CONTRACT_NAME" \
      --method=revokeCertificate \
      --sdk-conf-path="$CONF" \
      --params="{\"cert_id\":\"$CERT_ID\",\"revoke_reason\":\"信息填报错误\",\"operator_id\":\"edu_org_moe\"}" \
      --sync-result=true
    ;;

  *)
    echo "用法：bash deploy/test_contract.sh {query-student|query-cert|list|verify|revoke}"
    exit 1
    ;;
esac

echo
echo "提示：cmc 返回的 contract_result.result 是 base64，可直接复制到终端执行："
echo "      echo '<base64>' | base64 -d"
