#!/usr/bin/env bash
# 任务一：启动支持 Docker_VM 的长安链（4 节点，chain1，RPC 12301~12304）
#
# 用法：  bash deploy/01_start_chain.sh
# 前提：  已在虚拟机中编译好 chainmaker-go v2.3.4（build/release 已生成），
#         且本地已有 chainmaker-vm-engine:v2.3.4 镜像（首次启动会拉取，需要外网）。
set -euo pipefail

WORKDIR="${WORKDIR:-$HOME/chainmaker-lab}"
CHAIN_DIR="$WORKDIR/chainmaker-go"

echo "==> WORKDIR = $WORKDIR"

if [ ! -d "$CHAIN_DIR" ]; then
  echo "!! 未找到 $CHAIN_DIR，请先按 docs/01-环境与部署.md 安装并编译 chainmaker-go v2.3.4" >&2
  exit 1
fi

echo "==> 1) 检查是否已有节点在运行"
RUNNING=$(ps -ef | grep -c "[c]hainmaker" || true)
echo "    当前 chainmaker 进程数：$RUNNING（0 表示链未启动）"
if [ "$RUNNING" != "0" ]; then
  echo "    已有节点在运行。若要重启，先执行： pkill -9 chainmaker" >&2
fi

echo "==> 2) 检查 RPC 端口占用（12301~12304）"
for port in 12301 12302 12303 12304; do
  if ss -lnt 2>/dev/null | grep -q ":$port "; then
    echo "!! 端口 $port 已被占用：可能是上一轮的节点没退干净，先执行 pkill -9 chainmaker" >&2
    exit 1
  fi
done

echo "==> 3) 启动链（4 节点 + docker vm）"
cd "$CHAIN_DIR/scripts"
sudo ./cluster_quick_start.sh normal

echo "==> 4) 等待节点就绪"
sleep 5
ps -ef | grep "[c]hainmaker" | wc -l | xargs -I{} echo "    chainmaker 进程数：{}（期望 4）"

echo "==> 5) 检查合约引擎容器（DOCKER_GO）"
docker ps --format '{{.Names}}\t{{.Status}}' | grep 'VM-GO' || echo "    （暂无 VM-GO 容器，首次部署 DOCKER_GO 合约时会自动创建）"

echo
echo "链已启动：chain_id = chain1，RPC = 12301~12304"
echo "下一步： bash deploy/02_build_deploy_contract.sh"
