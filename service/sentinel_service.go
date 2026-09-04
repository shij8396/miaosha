package service

import (
	"sync"
	"sync/atomic"

	"github.com/miaosha/model"
)

// SentinelService Sentinel 规则管理服务（内存存储）
// 用于前端展示和管理 Sentinel 规则，实际规则生效由 sentinel-go 控制
type SentinelService struct {
	mu     sync.RWMutex
	rules  []model.SentinelRule
	nextID int64
}

var sentinelServiceInstance *SentinelService
var sentinelServiceOnce sync.Once

// GetSentinelService 获取 Sentinel 服务单例
func GetSentinelService() *SentinelService {
	sentinelServiceOnce.Do(func() {
		sentinelServiceInstance = &SentinelService{
			rules: []model.SentinelRule{
				{ID: 1, Resource: "seckill_api", Grade: 0, Count: 100, Strategy: 0, ControlBehavior: 0, LimitApp: "default"},
				{ID: 2, Resource: "global_api", Grade: 0, Count: 500, Strategy: 0, ControlBehavior: 0, LimitApp: "default"},
			},
			nextID: 3,
		}
	})
	return sentinelServiceInstance
}

// GetRules 获取所有规则
func (s *SentinelService) GetRules() []model.SentinelRule {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]model.SentinelRule, len(s.rules))
	copy(result, s.rules)
	return result
}

// AddRule 添加规则
func (s *SentinelService) AddRule(req *model.SentinelRuleRequest) *model.SentinelRule {
	s.mu.Lock()
	defer s.mu.Unlock()
	rule := model.SentinelRule{
		ID:                atomic.AddInt64(&s.nextID, 1) - 1,
		Resource:          req.Resource,
		Grade:             req.Grade,
		Count:             req.Count,
		Strategy:          req.Strategy,
		ControlBehavior:   req.ControlBehavior,
		LimitApp:          req.LimitApp,
		WarmUpPeriodSec:   req.WarmUpPeriodSec,
		MaxQueueingTimeMs: req.MaxQueueingTimeMs,
	}
	s.rules = append(s.rules, rule)
	return &rule
}

// DeleteRule 删除规则
func (s *SentinelService) DeleteRule(id int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, r := range s.rules {
		if r.ID == id {
			s.rules = append(s.rules[:i], s.rules[i+1:]...)
			return true
		}
	}
	return false
}

// [修复] UpdateRule 更新 Sentinel 规则
func (s *SentinelService) UpdateRule(id int64, req *model.SentinelRuleRequest) *model.SentinelRule {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, r := range s.rules {
		if r.ID == id {
			s.rules[i].Resource = req.Resource
			s.rules[i].Grade = req.Grade
			s.rules[i].Count = req.Count
			s.rules[i].Strategy = req.Strategy
			s.rules[i].ControlBehavior = req.ControlBehavior
			s.rules[i].LimitApp = req.LimitApp
			s.rules[i].WarmUpPeriodSec = req.WarmUpPeriodSec
			s.rules[i].MaxQueueingTimeMs = req.MaxQueueingTimeMs
			return &s.rules[i]
		}
	}
	return nil
}
