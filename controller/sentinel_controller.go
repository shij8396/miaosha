package controller

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/miaosha/model"
	"github.com/miaosha/service"
	"github.com/miaosha/utils"
)

type SentinelController struct {
	sentinelService *service.SentinelService
}

func NewSentinelController(sentinelService *service.SentinelService) *SentinelController {
	return &SentinelController{sentinelService: sentinelService}
}

// GetRules 获取所有 Sentinel 规则
func (ctl *SentinelController) GetRules(c *gin.Context) {
	rules := ctl.sentinelService.GetRules()
	utils.Success(c, rules)
}

// AddRule 添加 Sentinel 规则
func (ctl *SentinelController) AddRule(c *gin.Context) {
	var req model.SentinelRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	rule := ctl.sentinelService.AddRule(&req)
	utils.SuccessWithMessage(c, "添加成功", rule)
}

// DeleteRule 删除 Sentinel 规则
func (ctl *SentinelController) DeleteRule(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		utils.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	if !ctl.sentinelService.DeleteRule(id) {
		utils.NotFound(c, "规则不存在")
		return
	}
	utils.SuccessWithMessage(c, "删除成功", nil)
}

// [修复] UpdateRule 更新 Sentinel 规则
func (ctl *SentinelController) UpdateRule(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		utils.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	var req model.SentinelRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	rule := ctl.sentinelService.UpdateRule(id, &req)
	if rule == nil {
		utils.NotFound(c, "规则不存在")
		return
	}
	utils.SuccessWithMessage(c, "更新成功", rule)
}