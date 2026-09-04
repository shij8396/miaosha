package controller

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/miaosha/model"
	"github.com/miaosha/service"
	"github.com/miaosha/utils"
)

type BlacklistController struct {
	blacklistService *service.BlacklistService
}

func NewBlacklistController(blacklistService *service.BlacklistService) *BlacklistController {
	return &BlacklistController{blacklistService: blacklistService}
}

// GetBlacklist 获取黑名单列表
func (ctl *BlacklistController) GetBlacklist(c *gin.Context) {
	list, err := ctl.blacklistService.GetBlacklist()
	if err != nil {
		utils.InternalError(c, err.Error())
		return
	}
	utils.Success(c, list)
}

// AddBlacklist 添加黑名单
func (ctl *BlacklistController) AddBlacklist(c *gin.Context) {
	var req model.BlacklistRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	if err := ctl.blacklistService.AddBlacklist(&req); err != nil {
		utils.Error(c, 400, err.Error())
		return
	}
	utils.SuccessWithMessage(c, "添加成功", nil)
}

// RemoveBlacklist 移除黑名单
func (ctl *BlacklistController) RemoveBlacklist(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		utils.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	if err := ctl.blacklistService.RemoveBlacklist(id); err != nil {
		utils.Error(c, 400, err.Error())
		return
	}
	utils.SuccessWithMessage(c, "移除成功", nil)
}

// CheckBlacklist 检查是否在黑名单中
func (ctl *BlacklistController) CheckBlacklist(c *gin.Context) {
	typ := c.Query("type")
	value := c.Query("value")
	if typ == "" || value == "" {
		utils.BadRequest(c, "参数错误")
		return
	}
	exists, err := ctl.blacklistService.CheckBlacklist(typ, value)
	if err != nil {
		utils.InternalError(c, err.Error())
		return
	}
	utils.Success(c, gin.H{"exists": exists})
}
