// Package main 学历证书存证智能合约（ChainMaker v2.3.x / DOCKER_GO 运行时）
//
// 合约名：cc71（与 web/app.py 中的 CERT_CONTRACT 环境变量保持一致）
// 业务方法：
//
//	issueCertificate   颁发证书（高校 uni 角色）
//	queryCertificate   查询证书（支持 证书ID / 学生ID+姓名 两种方式，全部角色）
//	revokeCertificate  撤销证书（教育部 edu 角色）
//	listAllCertificates 全量证书台账（教育部 edu 角色）
//	verifyCertificate  证书哈希校验（扩展方法，任一角色）
//
// 说明：本合约在指导手册《基于长安链的学历证书存证》给出的示例代码基础上整理，
// 业务字段、状态键命名、双索引存储、审计日志与事件机制均与手册保持一致；
// listAllCertificates 改为“证书 ID 索引键”实现，避免不同 SDK 版本上状态迭代器
// 接口签名差异导致的编译问题，功能等价且不依赖全表扫描（详见 docs/04-与指导手册的差异说明.md）。
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"

	"chainmaker/pb/protogo"
	"chainmaker/sandbox"
	"chainmaker/sdk"
)

// 证书状态
const (
	StatusValid   = "valid"   // 有效
	StatusRevoked = "revoked" // 已撤销
)

// 状态命名空间（沿用指导手册的写法：第一个参数为状态命名空间）
const (
	certStore  = "certificates" // 证书数据
	auditStore = "audit_logs"   // 审计日志
)

// 索引键：记录全部证书 ID，供 listAllCertificates 使用
const indexKey = "index_certids"

// Certificate 完整证书结构（7 个核心业务字段，7 个链上元信息字段）
type Certificate struct {
	StudentID      string `json:"student_id"`      // 学生 ID
	Name           string `json:"name"`            // 学生姓名
	Education      string `json:"eduction"`        // 学历层次（本科/硕士等）；JSON 键沿用指导手册拼写 eduction，与前端表单一致
	SchoolName     string `json:"school_name"`     // 学校名称
	Major          string `json:"major"`           // 专业
	IssueDate      string `json:"issue_date"`      // 颁发日期（YYYYMMDD）
	GraduationDate string `json:"graduation_date"` // 毕业日期（YYYYMMDD）
	CertID         string `json:"cert_id"`         // 证书唯一 ID
	Hash           string `json:"hash"`            // 证书内容 SHA256，用于防篡改
	Status         string `json:"status"`          // 状态：valid / revoked
	TransactionID  string `json:"tx_id"`           // 颁发交易 ID
	IssuerOrg      string `json:"issuer_org"`      // 颁发机构
	RevokeReason   string `json:"revoke_reason"`   // 撤销原因（撤销后存在）
	RevokeTxID     string `json:"revoke_tx_id"`    // 撤销交易 ID（撤销后存在）
}

// AuditLog 审计日志（写入 audit_logs 命名空间，形成不可篡改操作台账）
type AuditLog struct {
	Operation     string `json:"operation"`    // ISSUE / QUERY / REVOKE
	UserID        string `json:"user_id"`      // 操作人
	OrgID         string `json:"org_id"`       // 操作人所属机构
	Details       string `json:"details"`      // 操作详情
	TransactionID string `json:"tx_id"`        // 关联交易 ID
	BlockHeight   int    `json:"block_height"` // 区块高度
}

// CertContract 证书存证合约
type CertContract struct{}

// InitContract 合约初始化
func (c *CertContract) InitContract() protogo.Response {
	if sdk.Instance == nil {
		return sdk.Error("SDK 实例未初始化")
	}
	return sdk.Success([]byte("学历证书存证合约初始化成功"))
}

// UpgradeContract 合约升级
func (c *CertContract) UpgradeContract() protogo.Response {
	return sdk.Success([]byte("学历证书存证合约升级成功"))
}

// InvokeContract 合约方法路由
func (c *CertContract) InvokeContract(method string) protogo.Response {
	if sdk.Instance == nil {
		return sdk.Error("合约调用失败：SDK 实例未初始化")
	}
	switch method {
	case "issueCertificate":
		return c.issueCertificate()
	case "queryCertificate":
		return c.queryCertificate()
	case "revokeCertificate":
		return c.revokeCertificate()
	case "listAllCertificates":
		return c.listAllCertificates()
	case "verifyCertificate":
		return c.verifyCertificate()
	default:
		return sdk.Error("无效方法：" + method)
	}
}

// ---------------------------------------------------------------- 颁发证书

func (c *CertContract) issueCertificate() protogo.Response {
	params := sdk.Instance.GetArgs()
	if params == nil {
		return sdk.Error("参数为空")
	}

	studentID := string(params["student_id"])
	name := string(params["name"])
	education := string(params["eduction"])
	schoolName := string(params["school_name"])
	major := string(params["major"])
	issueDate := string(params["issue_date"])
	graduationDate := string(params["graduation_date"])

	// 参数完整性校验
	if studentID == "" || name == "" || education == "" || schoolName == "" ||
		major == "" || issueDate == "" || graduationDate == "" {
		return sdk.Error("参数不完整，需包含：student_id、name、eduction、school_name、major、issue_date、graduation_date")
	}
	// 日期格式校验（YYYYMMDD）
	if !isDate8(issueDate) || !isDate8(graduationDate) {
		return sdk.Error("日期格式错误，issue_date 与 graduation_date 应为 YYYYMMDD（8 位数字）")
	}

	// 获取链上信息
	creatorOrg, err := sdk.Instance.GetCreatorOrgId()
	if err != nil || creatorOrg == "" {
		return sdk.Error("获取创建者机构 ID 失败：" + safeErrorMsg(err))
	}
	txID, err := sdk.Instance.GetTxId()
	if err != nil || txID == "" {
		return sdk.Error("获取交易 ID 失败：" + safeErrorMsg(err))
	}
	txIDPrefix := txID
	if len(txID) > 8 {
		txIDPrefix = txID[:8]
	}
	blockHeight, err := sdk.Instance.GetBlockHeight()
	if err != nil || blockHeight < 0 {
		return sdk.Error("获取区块高度失败：" + safeErrorMsg(err))
	}

	// 生成全局唯一证书 ID：cert_<学生ID>_<交易ID前 8 位>
	certID := fmt.Sprintf("cert_%s_%s", studentID, txIDPrefix)

	cert := Certificate{
		StudentID:      studentID,
		Name:           name,
		Education:      education,
		SchoolName:     schoolName,
		Major:          major,
		IssueDate:      issueDate,
		GraduationDate: graduationDate,
		CertID:         certID,
		Status:         StatusValid,
		TransactionID:  txID,
		IssuerOrg:      creatorOrg,
	}

	// 计算哈希：先把 hash 字段置空再做 SHA256，避免“哈希包含自身”
	cert.Hash = ""
	plain, err := json.Marshal(cert)
	if err != nil {
		return sdk.Error("证书序列化失败：" + safeErrorMsg(err))
	}
	cert.Hash = calculateHash(plain)

	certBytes, err := json.Marshal(cert)
	if err != nil {
		return sdk.Error("证书序列化失败：" + safeErrorMsg(err))
	}

	// 双键存储：学生信息索引 + 证书 ID 索引
	storageKeyByStudent := fmt.Sprintf("student_%s_name_%s", studentID, name)
	storageKeyByCertID := fmt.Sprintf("certid_%s", certID)
	if err := sdk.Instance.PutStateByte(certStore, storageKeyByStudent, certBytes); err != nil {
		return sdk.Error("存储学生证书索引失败：" + safeErrorMsg(err))
	}
	if err := sdk.Instance.PutStateByte(certStore, storageKeyByCertID, certBytes); err != nil {
		return sdk.Error("存储证书失败：" + safeErrorMsg(err))
	}
	// 全量台账索引
	if err := c.addToIndex(certID); err != nil {
		return sdk.Error("更新证书台账索引失败：" + safeErrorMsg(err))
	}

	// 审计日志 + 事件
	c.logAudit("ISSUE", studentID, creatorOrg, fmt.Sprintf("颁发证书：%s", certID), txID, blockHeight)
	sdk.Instance.EmitEvent("certificate_issued", []string{certID, studentID})

	return sdk.Success([]byte(fmt.Sprintf("证书颁发成功，证书ID：%s", certID)))
}

// ---------------------------------------------------------------- 查询证书

func (c *CertContract) queryCertificate() protogo.Response {
	params := sdk.Instance.GetArgs()
	if params == nil {
		return sdk.Error("参数为空")
	}

	studentID := string(params["student_id"])
	name := string(params["name"])
	certID := string(params["cert_id"])

	// 查询条件二选一
	if (studentID == "" || name == "") && certID == "" {
		return sdk.Error("查询参数不完整，需提供（student_id + name）或 cert_id")
	}
	if certID != "" && (studentID != "" || name != "") {
		return sdk.Error("证书 ID 与学生信息不能同时提供，请二选一查询")
	}

	var storageKey string
	if certID != "" {
		storageKey = fmt.Sprintf("certid_%s", certID)
	} else {
		storageKey = fmt.Sprintf("student_%s_name_%s", studentID, name)
	}

	certBytes, err := sdk.Instance.GetStateByte(certStore, storageKey)
	if err != nil {
		return sdk.Error("查询证书失败：" + safeErrorMsg(err))
	}
	if len(certBytes) == 0 {
		return sdk.Error("未找到对应证书：" + storageKey)
	}

	var cert Certificate
	if err := json.Unmarshal(certBytes, &cert); err != nil {
		return sdk.Error("证书解析失败：" + safeErrorMsg(err))
	}

	creatorOrg, _ := sdk.Instance.GetCreatorOrgId()
	txID, _ := sdk.Instance.GetTxId()
	blockHeight, _ := sdk.Instance.GetBlockHeight()
	c.logAudit("QUERY", studentID, creatorOrg, fmt.Sprintf("查询证书：%s", cert.CertID), txID, blockHeight)

	return sdk.Success(certBytes)
}

// ---------------------------------------------------------------- 撤销证书

func (c *CertContract) revokeCertificate() protogo.Response {
	params := sdk.Instance.GetArgs()
	if params == nil {
		return sdk.Error("参数为空")
	}

	certID := string(params["cert_id"])
	revokeReason := string(params["revoke_reason"])
	operatorID := string(params["operator_id"])

	if certID == "" || revokeReason == "" || operatorID == "" {
		return sdk.Error("参数不完整，需包含 cert_id、revoke_reason、operator_id")
	}

	storageKey := fmt.Sprintf("certid_%s", certID)
	certBytes, err := sdk.Instance.GetStateByte(certStore, storageKey)
	if err != nil {
		return sdk.Error("查询证书失败：" + safeErrorMsg(err))
	}
	if len(certBytes) == 0 {
		return sdk.Error("未找到证书：" + certID)
	}

	var cert Certificate
	if err := json.Unmarshal(certBytes, &cert); err != nil {
		return sdk.Error("证书解析失败：" + safeErrorMsg(err))
	}
	if cert.Status == StatusRevoked {
		return sdk.Error("证书已撤销：" + certID)
	}

	txID, err := sdk.Instance.GetTxId()
	if err != nil || txID == "" {
		return sdk.Error("获取交易 ID 失败：" + safeErrorMsg(err))
	}
	creatorOrg, _ := sdk.Instance.GetCreatorOrgId()
	blockHeight, _ := sdk.Instance.GetBlockHeight()

	// 更新状态与撤销信息
	cert.Status = StatusRevoked
	cert.RevokeReason = revokeReason
	cert.RevokeTxID = txID

	// 重新计算哈希（同样先把 hash 置空）
	cert.Hash = ""
	plain, err := json.Marshal(cert)
	if err != nil {
		return sdk.Error("更新后证书序列化失败：" + safeErrorMsg(err))
	}
	cert.Hash = calculateHash(plain)

	updatedCertBytes, err := json.Marshal(cert)
	if err != nil {
		return sdk.Error("更新后证书序列化失败：" + safeErrorMsg(err))
	}

	// 双键同步更新
	storageKeyByStudent := fmt.Sprintf("student_%s_name_%s", cert.StudentID, cert.Name)
	if err := sdk.Instance.PutStateByte(certStore, storageKey, updatedCertBytes); err != nil {
		return sdk.Error("更新证书失败：" + safeErrorMsg(err))
	}
	if err := sdk.Instance.PutStateByte(certStore, storageKeyByStudent, updatedCertBytes); err != nil {
		return sdk.Error("更新证书索引失败：" + safeErrorMsg(err))
	}

	c.logAudit("REVOKE", operatorID, creatorOrg,
		fmt.Sprintf("撤销证书：%s，原因：%s", certID, revokeReason), txID, blockHeight)
	sdk.Instance.EmitEvent("certificate_revoked", []string{certID, cert.StudentID})

	return sdk.Success([]byte(fmt.Sprintf("证书撤销成功，证书ID：%s", certID)))
}

// ---------------------------------------------------------------- 全量台账

// listAllCertificates 返回全部证书（按 CertID 去重、按证书 ID 升序）
func (c *CertContract) listAllCertificates() protogo.Response {
	ids := c.loadIndex()
	allCerts := make([]Certificate, 0, len(ids))

	for _, certID := range ids {
		certBytes, err := sdk.Instance.GetStateByte(certStore, fmt.Sprintf("certid_%s", certID))
		if err != nil || len(certBytes) == 0 {
			continue // 跳过异常数据
		}
		var cert Certificate
		if err := json.Unmarshal(certBytes, &cert); err != nil {
			continue // 跳过格式错误的数据
		}
		allCerts = append(allCerts, cert)
	}

	sort.Slice(allCerts, func(i, j int) bool { return allCerts[i].CertID < allCerts[j].CertID })

	resultBytes, err := json.Marshal(allCerts)
	if err != nil {
		return sdk.Error("序列化证书列表失败：" + safeErrorMsg(err))
	}
	return sdk.Success(resultBytes)
}

// ---------------------------------------------------------------- 哈希校验（扩展）

// verifyCertificate 重算链上证书内容的 SHA256 并与上链哈希比对，用于“真伪秒验”
func (c *CertContract) verifyCertificate() protogo.Response {
	params := sdk.Instance.GetArgs()
	certID := ""
	if params != nil {
		certID = string(params["cert_id"])
	}
	if certID == "" {
		return sdk.Error("参数不完整，需包含 cert_id")
	}

	certBytes, err := sdk.Instance.GetStateByte(certStore, fmt.Sprintf("certid_%s", certID))
	if err != nil {
		return sdk.Error("查询证书失败：" + safeErrorMsg(err))
	}
	if len(certBytes) == 0 {
		return sdk.Error("未找到证书：" + certID)
	}

	var cert Certificate
	if err := json.Unmarshal(certBytes, &cert); err != nil {
		return sdk.Error("证书解析失败：" + safeErrorMsg(err))
	}

	onChainHash := cert.Hash
	cert.Hash = ""
	plain, err := json.Marshal(cert)
	if err != nil {
		return sdk.Error("证书序列化失败：" + safeErrorMsg(err))
	}
	recomputed := calculateHash(plain)

	result := map[string]interface{}{
		"cert_id":         certID,
		"status":          cert.Status,
		"valid":           onChainHash == recomputed && cert.Status == StatusValid,
		"hash_on_chain":   onChainHash,
		"hash_recomputed": recomputed,
	}
	resultBytes, err := json.Marshal(result)
	if err != nil {
		return sdk.Error("序列化校验结果失败：" + safeErrorMsg(err))
	}
	return sdk.Success(resultBytes)
}

// ---------------------------------------------------------------- 内部工具

// loadIndex 读取证书 ID 索引
func (c *CertContract) loadIndex() []string {
	raw, err := sdk.Instance.GetStateByte(certStore, indexKey)
	if err != nil || len(raw) == 0 {
		return nil
	}
	var ids []string
	if err := json.Unmarshal(raw, &ids); err != nil {
		return nil
	}
	return ids
}

// addToIndex 把证书 ID 追加到台账索引（已存在则跳过）
func (c *CertContract) addToIndex(certID string) error {
	ids := c.loadIndex()
	for _, id := range ids {
		if id == certID {
			return nil
		}
	}
	ids = append(ids, certID)
	raw, err := json.Marshal(ids)
	if err != nil {
		return err
	}
	return sdk.Instance.PutStateByte(certStore, indexKey, raw)
}

// logAudit 记录审计日志：键为 <操作类型>_<交易ID>
func (c *CertContract) logAudit(operation, userID, orgID, details, txID string, blockHeight int) {
	if txID == "" {
		return
	}
	log := AuditLog{
		Operation:     operation,
		UserID:        userID,
		OrgID:         orgID,
		Details:       details,
		TransactionID: txID,
		BlockHeight:   blockHeight,
	}
	logBytes, err := json.Marshal(log)
	if err != nil {
		return
	}
	sdk.Instance.PutStateByte(auditStore, fmt.Sprintf("%s_%s", operation, txID), logBytes)
}

// calculateHash 计算 SHA256 十六进制摘要
func calculateHash(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// isDate8 校验 YYYYMMDD 格式
func isDate8(s string) bool {
	if len(s) != 8 {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// safeErrorMsg 安全提取错误信息
func safeErrorMsg(err error) string {
	if err == nil {
		return "未知错误"
	}
	return err.Error()
}

func main() {
	if err := sandbox.Start(new(CertContract)); err != nil {
		fmt.Printf("合约启动失败：%v\n", err)
	}
}
