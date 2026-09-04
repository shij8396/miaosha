package service

import (
	"errors"
	"fmt"
	"log"

	"github.com/miaosha/dao"
	"github.com/miaosha/model"
	"github.com/miaosha/utils"
	"gorm.io/gorm"
)

type UserService struct {
	jwtManager *utils.JWTManager
}

func NewUserService(jwtManager *utils.JWTManager) *UserService {
	return &UserService{jwtManager: jwtManager}
}

func (s *UserService) Register(req *model.RegisterRequest) (*model.User, error) {
	existing, err := dao.GetUserByUsername(req.Username)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		// [修复] DB 故障时不再误判为"用户名不存在"继续走创建分支
		log.Printf("[REGISTER] 查询用户失败 username=%s err=%v", req.Username, err)
		return nil, fmt.Errorf("系统繁忙，请稍后再试")
	}
	if existing != nil {
		return nil, fmt.Errorf("用户名已存在")
	}
	hashedPassword, err := utils.HashPassword(req.Password)
	if err != nil {
		return nil, fmt.Errorf("密码加密失败: %w", err)
	}
	user := &model.User{Username: req.Username, Password: hashedPassword, Nickname: req.Nickname, Phone: req.Phone, Status: 1}
	if err := dao.CreateUser(user); err != nil {
		return nil, fmt.Errorf("创建用户失败: %w", err)
	}
	return user, nil
}

func (s *UserService) Login(req *model.LoginRequest) (*model.LoginResponse, error) {
	user, err := dao.GetUserByUsername(req.Username)
	if err != nil {
		// [修复] 区分"用户不存在"与"DB故障"：DB故障记录真实错误，避免被统一文案掩盖
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("[LOGIN] 查询用户失败 username=%s err=%v", req.Username, err)
		}
		return nil, fmt.Errorf("用户名或密码错误")
	}
	if user.Status != 1 {
		return nil, fmt.Errorf("账号已被禁用")
	}
	if !utils.CheckPassword(req.Password, user.Password) {
		return nil, fmt.Errorf("用户名或密码错误")
	}
	token, err := s.jwtManager.GenerateToken(user.ID, user.Username, user.Role) // [修复] 传递角色信息到JWT
	if err != nil {
		return nil, fmt.Errorf("Token生成失败: %w", err)
	}
	return &model.LoginResponse{Token: token, UserID: user.ID, Username: user.Username, Nickname: user.Nickname, Role: user.Role}, nil
}

// GetUserList 获取用户列表（管理员）
func (s *UserService) GetUserList(page, pageSize int) ([]model.UserListResponse, int64, error) {
	users, total, err := dao.GetUserList(page, pageSize)
	if err != nil {
		return nil, 0, err
	}
	list := make([]model.UserListResponse, 0, len(users))
	for _, u := range users {
		list = append(list, model.UserListResponse{
			ID: u.ID, Username: u.Username, Nickname: u.Nickname,
			Phone: u.Phone, Role: u.Role, Status: u.Status, CreatedAt: u.CreatedAt,
		})
	}
	return list, total, nil
}

// UpdateUserRole 更新用户角色
func (s *UserService) UpdateUserRole(req *model.UpdateUserRoleRequest) error {
	return dao.UpdateUserRole(req.UserID, req.Role)
}

// GetUserInfo 获取当前用户信息
func (s *UserService) GetUserInfo(userID int64) (*model.User, error) {
	user, err := dao.GetUserByID(userID)
	if err != nil {
		return nil, fmt.Errorf("用户不存在")
	}
	return user, nil
}

// [修复] ChangePassword 修改密码：验证旧密码后更新为新密码
func (s *UserService) ChangePassword(userID int64, oldPassword, newPassword string) error {
	user, err := dao.GetUserByID(userID)
	if err != nil {
		return fmt.Errorf("用户不存在")
	}
	if !utils.CheckPassword(oldPassword, user.Password) {
		return fmt.Errorf("旧密码错误")
	}
	hashedPassword, err := utils.HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("密码加密失败")
	}
	if err := dao.UpdatePassword(userID, hashedPassword); err != nil {
		return fmt.Errorf("密码修改失败: %w", err)
	}
	return nil
}

// ForgotPassword 用户忘记密码：通过用户名+手机号验证身份后重置密码
func (s *UserService) ForgotPassword(req *model.ForgotPasswordRequest) error {
	user, err := dao.GetUserByUsername(req.Username)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("用户名不存在")
		}
		return fmt.Errorf("系统繁忙，请稍后再试")
	}
	if user.Phone == "" {
		return fmt.Errorf("该账号未绑定手机号，请联系管理员")
	}
	if user.Phone != req.Phone {
		return fmt.Errorf("手机号与注册时不一致，验证失败")
	}
	if user.Status != 1 {
		return fmt.Errorf("账号已被禁用")
	}
	hashedPassword, err := utils.HashPassword(req.NewPassword)
	if err != nil {
		return fmt.Errorf("密码加密失败")
	}
	if err := dao.UpdatePassword(user.ID, hashedPassword); err != nil {
		return fmt.Errorf("密码重置失败: %w", err)
	}
	return nil
}
