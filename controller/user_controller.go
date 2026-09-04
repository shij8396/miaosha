package controller

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/miaosha/model"
	"github.com/miaosha/service"
	"github.com/miaosha/utils"
)

type UserController struct {
	userService *service.UserService
}

func NewUserController(userService *service.UserService) *UserController {
	return &UserController{userService: userService}
}

// Register 用户注册
// @Summary      用户注册
// @Description  创建新用户账号，密码使用 bcrypt 加密存储
// @Tags         用户模块
// @Accept       json
// @Produce      json
// @Param        request body model.RegisterRequest true "注册请求"
// @Success      200  {object}  utils.Response{data=object{user_id=int,username=string}}  "注册成功"
// @Failure      400  {object}  utils.Response  "参数错误或用户名已存在"
// @Router       /api/v1/user/register [post]
func (ctl *UserController) Register(c *gin.Context) {
	var req model.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil { utils.BadRequest(c, "参数错误: "+err.Error()); return }
	user, err := ctl.userService.Register(&req)
	if err != nil { utils.Error(c, 400, err.Error()); return }
	utils.SuccessWithMessage(c, "注册成功", gin.H{"user_id": user.ID, "username": user.Username})
}

// Login 用户登录
// @Summary      用户登录
// @Description  使用用户名和密码登录，返回 JWT Token，有效期 24 小时
// @Tags         用户模块
// @Accept       json
// @Produce      json
// @Param        request body model.LoginRequest true "登录请求"
// @Success      200  {object}  utils.Response{data=model.LoginResponse}  "登录成功"
// @Failure      400  {object}  utils.Response  "用户名或密码错误"
// @Failure      429  {object}  utils.Response  "登录尝试过于频繁"
// @Router       /api/v1/user/login [post]
func (ctl *UserController) Login(c *gin.Context) {
	var req model.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil { utils.BadRequest(c, "参数错误: "+err.Error()); return }
	resp, err := ctl.userService.Login(&req)
	if err != nil { utils.Error(c, 400, err.Error()); return }
	utils.Success(c, resp)
}

// GetUserList 获取用户列表（管理员）
// @Summary      获取用户列表
// @Description  管理员分页查询所有用户，支持按角色筛选
// @Tags         用户模块
// @Accept       json
// @Produce      json
// @Param        page      query     int  false  "页码"  default(1)
// @Param        page_size query     int  false  "每页数量"  default(10)
// @Security     BearerAuth
// @Success      200  {object}  utils.Response{data=object{list=[]model.UserListResponse,total=int,page=int,page_size=int}}  "查询成功"
// @Failure      401  {object}  utils.Response  "未登录"
// @Router       /api/v1/user/list [get]
func (ctl *UserController) GetUserList(c *gin.Context) {
	page := 1
	pageSize := 10
	if p, ok := c.GetQuery("page"); ok { fmt.Sscanf(p, "%d", &page) }
	if ps, ok := c.GetQuery("page_size"); ok { fmt.Sscanf(ps, "%d", &pageSize) }
	// [修复] 分页上限限制
	pageSize = utils.ClampPageSize(pageSize)
	list, total, err := ctl.userService.GetUserList(page, pageSize)
	if err != nil { utils.InternalError(c, err.Error()); return }
	utils.Success(c, gin.H{"list": list, "total": total, "page": page, "page_size": pageSize})
}

// UpdateUserRole 更新用户角色
// @Summary      更新用户角色
// @Description  管理员修改用户角色（super_admin/operator/monitor_readonly/risk_control/admin/user）
// @Tags         用户模块
// @Accept       json
// @Produce      json
// @Param        request body model.UpdateUserRoleRequest true "角色更新请求"
// @Security     BearerAuth
// @Success      200  {object}  utils.Response  "角色更新成功"
// @Failure      400  {object}  utils.Response  "参数错误"
// @Failure      401  {object}  utils.Response  "未登录"
// @Router       /api/v1/user/role [put]
func (ctl *UserController) UpdateUserRole(c *gin.Context) {
	var req model.UpdateUserRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil { utils.BadRequest(c, "参数错误: "+err.Error()); return }
	if err := ctl.userService.UpdateUserRole(&req); err != nil { utils.Error(c, 400, err.Error()); return }
	utils.SuccessWithMessage(c, "角色更新成功", nil)
}

// GetUserInfo 获取当前用户信息
// @Summary      获取当前用户信息
// @Description  获取当前登录用户的详细信息，包括角色、状态等
// @Tags         用户模块
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  utils.Response{data=object{user_id=int,username=string,nickname=string,phone=string,role=string,status=int,created_at=string}}  "查询成功"
// @Failure      401  {object}  utils.Response  "未登录"
// @Router       /api/v1/user/info [get]
func (ctl *UserController) GetUserInfo(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists { utils.Unauthorized(c, "请先登录"); return }
	uid := userID.(int64)
	user, err := ctl.userService.GetUserInfo(uid)
	if err != nil { utils.NotFound(c, err.Error()); return }
	utils.Success(c, gin.H{
		"user_id": user.ID, "username": user.Username, "nickname": user.Nickname,
		"phone": user.Phone, "role": user.Role, "status": user.Status, "created_at": user.CreatedAt,
	})
}

// [修复] ChangePassword 修改密码 - PUT /api/v1/user/password
// 需要验证旧密码，新密码明文传输后由后端 bcrypt 加密
// @Summary      修改密码
// @Description  验证旧密码后修改为新密码，修改成功后需重新登录
// @Tags         用户模块
// @Accept       json
// @Produce      json
// @Param        request body object{old_password=string,new_password=string} true "密码修改请求"
// @Security     BearerAuth
// @Success      200  {object}  utils.Response  "密码修改成功"
// @Failure      400  {object}  utils.Response  "旧密码错误或参数错误"
// @Failure      401  {object}  utils.Response  "未登录"
// @Router       /api/v1/user/password [put]
func (ctl *UserController) ChangePassword(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists { utils.Unauthorized(c, "请先登录"); return }
	uid := userID.(int64)
	var req struct {
		OldPassword string `json:"old_password" binding:"required"`
		NewPassword string `json:"new_password" binding:"required,min=6"`
	}
	if err := c.ShouldBindJSON(&req); err != nil { utils.BadRequest(c, "参数错误: "+err.Error()); return }
	if err := ctl.userService.ChangePassword(uid, req.OldPassword, req.NewPassword); err != nil {
		utils.Error(c, 400, err.Error())
		return
	}
	utils.SuccessWithMessage(c, "密码修改成功", nil)
}

// ForgotPassword 用户忘记密码 - POST /api/v1/user/forgot-password
// 通过用户名 + 注册手机号验证身份后重置密码
// @Summary      忘记密码
// @Description  用户忘记密码时，通过用户名和注册手机号验证身份后重置新密码
// @Tags         用户模块
// @Accept       json
// @Produce      json
// @Param        request body model.ForgotPasswordRequest true "忘记密码请求"
// @Success      200  {object}  utils.Response  "密码重置成功"
// @Failure      400  {object}  utils.Response  "参数错误或验证失败"
// @Router       /api/v1/user/forgot-password [post]
func (ctl *UserController) ForgotPassword(c *gin.Context) {
	var req model.ForgotPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil { utils.BadRequest(c, "参数错误: "+err.Error()); return }
	if err := ctl.userService.ForgotPassword(&req); err != nil {
		utils.Error(c, 400, err.Error())
		return
	}
	utils.SuccessWithMessage(c, "密码重置成功，请使用新密码登录", nil)
}