# CertContract · 学历证书存证合约

`main.go` 是提交给 **SmartIDE**（<https://ide.chainmaker.org.cn/>）编译的合约源码。

## 编译步骤

1. SmartIDE 新建 Go 合约工程，工程名建议 `CertContract`。
2. 用本文件全文替换工程里的 `main.go`，保存。
3. 点击「编译 / 构建」，等编译成功提示。
4. 下载编译产物（`7z` 包），上传到虚拟机：

```bash
mkdir -p ~/chainmaker-lab/chainmaker-go/tools/cmc/testdata/claim-docker-go-demo
# 把 cc71.7z 放到该目录
```

## 依赖的合约 SDK 接口

`chainmaker/sdk`（`sdk.Instance`）提供的：`GetArgs`、`GetStateByte`、`PutStateByte`、
`GetCreatorOrgId`、`GetTxId`、`GetBlockHeight`、`EmitEvent`；入口为 `chainmaker/sandbox` 的
`sandbox.Start`。这些接口在 ChainMaker v2.3.x 的 DOCKER_GO 运行时下可用。

> 如果你使用的合约 SDK 是较老的 `chainmaker/shim`（`stub shim.CMStubInterface`）风格，
> 只需把方法签名改为 `func (c *CertContract) InvokeContract(stub shim.CMStubInterface) protogo.Response`，
> 并把 `sdk.Instance.GetArgs()` → `stub.GetArgs()`、
> `sdk.Instance.GetStateByte(ns, key)` → `stub.GetStateFromKey(ns+"#"+key)` 之类的调用对齐即可；
> 业务逻辑无需改动。

## 方法一览

| 方法 | 类型 | 说明 |
| --- | --- | --- |
| `issueCertificate` | 写 | 颁发证书，生成证书 ID、计算 SHA256、双索引写入 |
| `queryCertificate` | 读/写 | 按证书 ID 或学生 ID + 姓名查询 |
| `revokeCertificate` | 写 | 撤销证书，记录原因与撤销交易 ID，重算哈希 |
| `listAllCertificates` | 读 | 全量台账（按索引去重、`cert_id` 升序） |
| `verifyCertificate` | 读 | 重算 SHA256 并与链上哈希比对 |

参数与返回示例见 [docs/03-合约与接口说明.md](../../docs/03-合约与接口说明.md)。
