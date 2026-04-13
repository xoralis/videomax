package handler

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"video-max/internal/domain/dto"
	"video-max/internal/domain/entity"
	"video-max/internal/repository"
)

// AuthHandler 用户注册/登录相关接口控制器
type AuthHandler struct {
	userRepo   repository.UserRepository
	jwtSecret  string
	expireDays int
}

// NewAuthHandler 创建 AuthHandler 实例
func NewAuthHandler(userRepo repository.UserRepository, jwtSecret string, expireDays int) *AuthHandler {
	days := expireDays
	if days <= 0 {
		days = 7
	}
	return &AuthHandler{
		userRepo:   userRepo,
		jwtSecret:  jwtSecret,
		expireDays: days,
	}
}

// Register 用户注册
// POST /api/auth/register
func (h *AuthHandler) Register(c *gin.Context) {
	var req dto.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.AuthResponse{Code: -1, Msg: "参数校验失败: " + err.Error()})
		return
	}

	// 检查邮箱是否已注册
	_, err := h.userRepo.GetByEmail(c.Request.Context(), req.Email)
	if err == nil {
		c.JSON(http.StatusConflict, dto.AuthResponse{Code: -1, Msg: "该邮箱已被注册"})
		return
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusInternalServerError, dto.AuthResponse{Code: -1, Msg: "服务器内部错误"})
		return
	}

	// bcrypt 哈希密码
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.AuthResponse{Code: -1, Msg: "密码处理失败"})
		return
	}

	user := &entity.User{
		UserID:       uuid.New().String(),
		Username:     req.Username,
		Email:        req.Email,
		PasswordHash: string(hash),
	}

	if err := h.userRepo.Create(c.Request.Context(), user); err != nil {
		c.JSON(http.StatusInternalServerError, dto.AuthResponse{Code: -1, Msg: "注册失败: " + err.Error()})
		return
	}

	token, err := h.generateToken(user.UserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.AuthResponse{Code: -1, Msg: "Token 生成失败"})
		return
	}

	c.JSON(http.StatusOK, dto.AuthResponse{
		Code:     0,
		Token:    token,
		UserID:   user.UserID,
		Username: user.Username,
		Email:    user.Email,
		Msg:      "注册成功",
	})
}

// Login 用户登录
// POST /api/auth/login
func (h *AuthHandler) Login(c *gin.Context) {
	var req dto.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.AuthResponse{Code: -1, Msg: "参数校验失败: " + err.Error()})
		return
	}

	user, err := h.userRepo.GetByEmail(c.Request.Context(), req.Email)
	if err != nil {
		c.JSON(http.StatusUnauthorized, dto.AuthResponse{Code: -1, Msg: "邮箱或密码错误"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, dto.AuthResponse{Code: -1, Msg: "邮箱或密码错误"})
		return
	}

	token, err := h.generateToken(user.UserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.AuthResponse{Code: -1, Msg: "Token 生成失败"})
		return
	}

	c.JSON(http.StatusOK, dto.AuthResponse{
		Code:     0,
		Token:    token,
		UserID:   user.UserID,
		Username: user.Username,
		Email:    user.Email,
		Msg:      "登录成功",
	})
}

// generateToken 生成 JWT Token
func (h *AuthHandler) generateToken(userID string) (string, error) {
	claims := jwt.MapClaims{
		"user_id": userID,
		"exp":     time.Now().Add(time.Duration(h.expireDays) * 24 * time.Hour).Unix(),
		"iat":     time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(h.jwtSecret))
}
