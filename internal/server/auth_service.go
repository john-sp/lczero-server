package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	pb "github.com/leelachesszero/lczero-server/api/v1"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	model "github.com/leelachesszero/lczero-server/internal/models"

	"gorm.io/gorm"
)

// Lots of this code will to be updated to work with dev.lczero.org's for token system.

// AuthServiceServer defines the server interface for AuthService.
// This mirrors the gRPC-generated interface and allows us to start structuring the code
// before wiring protoc generation.
type AuthServiceServer interface {
	MigrateCredentials(ctx context.Context, req *pb.MigrateCredentialsRequest) (*pb.AuthResponse, error)
	GetAnonymousToken(ctx context.Context, req *pb.AnonymousTokenRequest) (*pb.AuthResponse, error)
}

// AuthServiceImpl is the concrete implementation backed by Gorm and our models.
type AuthServiceImpl struct {
	pb.UnimplementedAuthServiceServer
	DB *gorm.DB
}

// generateUniqueToken generates a unique token with the prefix "lc0-" and 64 random characters.
// It checks the DB to ensure the token does not already exist.
func generateUniqueToken(db *gorm.DB) (string, error) {
	const (
		prefix      = "lc0-"
		tokenLen    = 64
		maxAttempts = 10
	)

	for range maxAttempts {
		raw := make([]byte, tokenLen/2) // 32 bytes = 64 hex chars
		_, err := rand.Read(raw)
		if err != nil {
			return "", err
		}
		token := prefix + hex.EncodeToString(raw)

		var count int64
		err = db.Model(&model.AuthToken{}).Where("token = ?", token).Count(&count).Error
		if err != nil {
			return "", err
		}
		if count == 0 {
			return token, nil
		}
	}
	return "", errors.New("could not generate unique token after several attempts")
}

// NewAuthService creates a new AuthServiceImpl.
func NewAuthService(dbConn *gorm.DB) *AuthServiceImpl {
	return &AuthServiceImpl{DB: dbConn}
}

// MigrateCredentials exchanges a legacy username/password for a token.
func (s *AuthServiceImpl) MigrateCredentials(ctx context.Context, req *pb.MigrateCredentialsRequest) (*pb.AuthResponse, error) {

	// Validate username and password
	if len(req.Username) == 0 {
		return nil, status.Error(codes.InvalidArgument, "No username supplied")
	}
	if len(req.Username) > 32 {
		return nil, status.Error(codes.InvalidArgument, "Username too long")
	}
	if len(req.Password) == 0 {
		return nil, status.Error(codes.InvalidArgument, "No password supplied")
	}

	// Look up user
	user := &model.User{
		Username: req.Username,
		Password: req.Password,
	}
	err := s.DB.Where(model.User{Username: req.Username}).First(user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, status.Error(codes.NotFound, "User not found")
		}
		return nil, status.Error(codes.Internal, "Database error")
	}

	tokenStr, err := generateUniqueToken(s.DB)
	if err != nil {
		return nil, status.Error(codes.Internal, "Failed to generate token")
	}
	token := &model.AuthToken{
		Token:        tokenStr,
		IssuedReason: model.TokenReasonMigrated,
	}
	now := time.Now()
	token.CreatedAt = now
	if err := s.DB.Create(token).Error; err != nil {
		return nil, err
	}
	return &pb.AuthResponse{Token: token.Token}, nil
}

// GetAnonymousToken issues an anonymous token without a user.
func (s *AuthServiceImpl) GetAnonymousToken(ctx context.Context, req *pb.AnonymousTokenRequest) (*pb.AuthResponse, error) {
	tokenStr, err := generateUniqueToken(s.DB)
	if err != nil {
		return nil, status.Error(codes.Internal, "Failed to generate token")
	}
	token := &model.AuthToken{
		Token:        tokenStr,
		IssuedReason: model.TokenReasonAnonymous,
	}
	now := time.Now()
	token.CreatedAt = now
	if err := s.DB.Create(token).Error; err != nil {
		return nil, err
	}
	return &pb.AuthResponse{Token: token.Token}, nil
}
