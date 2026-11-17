# 基于长安链的学历证书存证系统

这是我做的一个实验项目，用长安链做一个学历证书存证的小系统。证书信息写到链上，
网页上可以签发、查询、验证和撤销证书。

## 我做了什么

- 写了一个 Go 智能合约，代码在 `contracts/CertContract/main.go`
- 用 Flask 写了一个简单网页，代码在 `web/`
- 网页通过 `cmc` 命令调用链上的合约
- 测试了签发、查询、验真和撤销这几个功能

## 用的环境

- Ubuntu 22.04
- Go 1.18.10
- ChainMaker v2.3.4
- Docker
- Python 3.10
- Flask 3.x

## 合约里的方法

- `issueCertificate`：签发证书，生成证书 ID，计算证书内容的 SHA256
- `queryCertificate`：按证书 ID 或者“学生 ID + 姓名”查询
- `verifyCertificate`：重新计算摘要，和链上的摘要比较
- `revokeCertificate`：撤销证书，把状态改成 `revoked`
- `listAllCertificates`：查看所有证书

证书上主要存这些内容：学号、姓名、学校、专业、学历、颁发日期、毕业日期、
证书 ID、摘要、状态、交易 ID、颁发机构。

## 怎么跑

### 1. 启动链

```bash
cd ~/chainmaker-lab/chainmaker-go/scripts
sudo ./cluster_quick_start.sh normal
```

### 2. 部署合约

合约需要先在 SmartIDE 里编译成 `.7z` 文件，再放到 cmc 的 testdata 目录。
部署命令可以参考 `deploy/02_build_deploy_contract.sh`。

我部署时用的合约名是：

```text
Web777
```

如果换成别的名字，后面的 `app.py` 和 deploy 脚本里的合约名也要一起改。

### 3. 启动网页

```bash
cd web
python3 -m pip install -r requirements.txt

export CERT_CONTRACT=Web777
python3 app.py
```

浏览器打开：

```text
http://<虚拟机IP>:5000
```

## 测试账号

| 角色 | 用户名 | 密码 |
| --- | --- | --- |
| 查询机构 | `query_org` | `Query@2024` |
| 高校 | `uni_org_bupt` | `Uni@2024` |
| 教育部 | `edu_org_moe` | `Edu@2024` |

## 测试过程

1. 用高校账号登录，签发一张证书
2. 用查询机构账号登录，按证书 ID 查询
3. 用正确摘要验证，应该通过
4. 把摘要改掉再验证，应该不通过
5. 用教育部账号撤销证书
6. 撤销后再查询，状态变成 `revoked`
7. 撤销后再验证，应该不通过

## 遇到的问题

1. 合约一开始部署成功，但一调用就 `runtime panic`。后来发现是编译环境的问题，
   换成 Go 1.18.10 重新编译后才正常。
2. 中文姓名不能直接拼到链上的 key 里，会报 key 不合法。后来学生索引改成用学号，
   不再把姓名拼进 key。
3. 网页里调用 `cmc` 时，如果不在 cmc 目录下执行，会找不到证书文件。后来在代码里
   指定了工作目录。
4. 合约名、证书路径、`sdk_config.yml` 里的路径只要有一个不对，就会调用失败。
   这几个地方要统一。

## 说明

这是实验课的作业，主要是按实验指导书做的。合约和网页在指导书例子的基础上做了一些
修改，比如修了中文 key 的问题、换了编译环境、加了验真方法。代码可能还有不完善的地方。
