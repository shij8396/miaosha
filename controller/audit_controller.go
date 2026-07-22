package controller

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/miaosha/model"
	"github.com/miaosha/service"
	"github.com/miaosha/utils"
)

type AuditController struct {
	auditService *service.AuditService
}

func NewAuditController(auditService *service.AuditService) *AuditController {
	return &AuditController{auditService: auditService}
}

// GetAuditLogs 查询审计日志 - GET /api/v1/audit/list
func (ctl *AuditController) GetAuditLogs(c *gin.Context) {
	page := 1
	pageSize := 10
	var operatorID int64
	var action string
	if p, ok := c.GetQuery("page"); ok { fmt.Sscanf(p, "%d", &page) }
	if ps, ok := c.GetQuery("page_size"); ok { fmt.Sscanf(ps, "%d", &pageSize) }
	// [修复] 分页上限限制
	pageSize = utils.ClampPageSize(pageSize)
	if oid, ok := c.GetQuery("operator_id"); ok { fmt.Sscanf(oid, "%d", &operatorID) }
	action = c.Query("action")
	list, total, err := ctl.auditService.GetAuditLogs(page, pageSize, operatorID, action)
	if err != nil { utils.InternalError(c, err.Error()); return }
	utils.Success(c, gin.H{"list": list, "total": total, "page": page, "page_size": pageSize})
}

// [修复] CreateAuditLog 供内部调用的审计日志写入方法
func WriteAuditLog(operatorID int64, operatorName, action, module, detail, clientIP string) {
	auditService := service.NewAuditService()
	_ = auditService.CreateAuditLog(&model.AuditLog{
		UserID:   operatorID,
		Username: operatorName,
		Action:   action,
		Module:   module,
		Detail:   detail,
		ClientIP: clientIP,
	})
}