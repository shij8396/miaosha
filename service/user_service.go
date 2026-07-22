package service

import (
	"fmt"
	"github.com/miaosha/dao"
	"github.com/miaosha/model"
	"github.com/miaosha/utils"
)

type UserService struct {
	jwtManager *utils.JWTManager
}

func NewUserService(jwtManager *utils.JWTManager) *UserService {
	return &UserService{jwtManager: jwtManager}
}

func (s *UserService) Register(req *model.RegisterRequest) (*model.User, error) {
	existing, _ := dao.GetUserByUsername(req.Username)
	if existing != nil { return nil, fmt.Errorf("用户名已存在") }
	hashedPassword, err := utils.HashPassword(req.Password)
	if err != nil { return nil, fmt.Errorf("密码加密失败: %w", err) }
	user := &model.User{Username: req.Username, Password: hashedPassword, Nickname: req.Nickname, Phone: req.Phone, Status: 1}
	if err := dao.CreateUser(user); err != nil { return nil, fmt.Errorf("创建用户失败: %w", err) }
	return user, nil
}

func (s *UserService) Login(req *model.LoginRequest) (*model.LoginResponse, error) {
	user, err := dao.GetUserByUsername(req.Username)
	if err != nil { return nil, fmt.Errorf("用户名或密码错误") }
	if user.Status != 1 { return nil, fmt.Errorf("账号已被禁用") }
	if !utils.CheckPassword(req.Password, user.Password) { return nil, fmt.Errorf("用户名或密码错误") }
	token, err := s.jwtManager.GenerateToken(user.ID, user.Username, user.Role) // [修复] 传递角色信息到JWT
	if err != nil { return nil, fmt.Errorf("Token生成失败: %w", err) }
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