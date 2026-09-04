package service

import (
	"github.com/miaosha/dao"
	"github.com/miaosha/model"
)

type AuditService struct{}

func NewAuditService() *AuditService {
	return &AuditService{}
}

// CreateAuditLog 写入操作审计日志
func (s *AuditService) CreateAuditLog(log *model.AuditLog) error {
	return dao.CreateAuditLog(log)
}

// GetAuditLogs 分页查询审计日志
func (s *AuditService) GetAuditLogs(page, pageSize int, operatorID int64, action string) ([]model.AuditLog, int64, error) {
	return dao.GetAuditLogs(page, pageSize, operatorID, action)
}
