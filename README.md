# 基于长安链的学历证书存证系统

这是我根据实验书做的最终大作业：用长安链存学历证书。证书信息写到链上，网页上可以签发、查询、验证和撤销。

## 内容

- 合约在 `contracts/CertContract/main.go`，用 Go 写的。
- 网页在 `web/`，用 Flask 写的。
- 网页通过 cmc 调合约。
- 试了签发、查询、验真、撤销。

## 合约方法

- `issueCertificate` 签发，生成证书 ID，算 SHA256。
- `queryCertificate` 按证书 ID 或学号+姓名查。
- `verifyCertificate` 重新算摘要，和链上比。
- `revokeCertificate` 撤销，状态改成 revoked。
- `listAllCertificates` 看全部证书。

证书里大概存学号、姓名、学校、专业、学历、日期、证书 ID、摘要、状态等。

## 步骤

先启动链：

```bash
cd ~/chainmaker-lab/chainmaker-go/scripts
sudo ./cluster_quick_start.sh normal
```

合约要先在 SmartIDE 编成 7z，放到 cmc 的 testdata 目录，再部署。部署命令在 `deploy/02_build_deploy_contract.sh` 里。

我用的合约名是 `You777`，换成别的名字的话，`app.py` 和 deploy 脚本里的名字也要改。

启动网页：

```bash
cd web
python3 -m pip install -r requirements.txt
export CERT_CONTRACT=You777
python3 app.py
```

然后浏览器打开 `http://虚拟机IP:5000`。

## 账号

| 角色 | 用户名 | 密码 |
| --- | --- | --- |
| 查询机构 | query_org | Query@2024 |
| 高校 | uni_org_bupt | Uni@2024 |
| 教育部 | edu_org_moe | Edu@2024 |

## 说明

这是实验课的作业，主要是按实验指导书做的。合约和网页在指导书例子的基础上做了一些
修改，比如修了中文 key 的问题、换了编译环境、加了验真方法。代码可能还有不完善的地方。
