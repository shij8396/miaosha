package service

import (
	"github.com/miaosha/dao"
	"github.com/miaosha/model"
)

type BlacklistService struct{}

func NewBlacklistService() *BlacklistService {
	return &BlacklistService{}
}

// GetBlacklist 获取黑名单列表
func (s *BlacklistService) GetBlacklist() ([]model.Blacklist, error) {
	return dao.GetBlacklist()
}

// AddBlacklist 添加黑名单
func (s *BlacklistService) AddBlacklist(req *model.BlacklistRequest) error {
	bl := &model.Blacklist{
		Type:   req.Type,
		Value:  req.Value,
		Reason: req.Reason,
	}
	return dao.CreateBlacklist(bl)
}

// RemoveBlacklist 移除黑名单
func (s *BlacklistService) RemoveBlacklist(id int64) error {
	return dao.DeleteBlacklist(id)
}

// CheckBlacklist 检查是否在黑名单中
func (s *BlacklistService) CheckBlacklist(typ, value string) (bool, error) {
	return dao.CheckBlacklist(typ, value)
}