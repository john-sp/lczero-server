package server

import (
	"context"
	"errors"
	"time"

	pb "github.com/leelachesszero/lczero-server/api/v1"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	model "github.com/leelachesszero/lczero-server/internal/models"

	"gorm.io/gorm"
)

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

	// TODO: fill implementation to issue token
	_ = req
	token := &model.AuthToken{
		Token:        "TODO-generate",
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
	// TODO: fill implementation (create token with null user, expiry policy)
	_ = req
	token := &model.AuthToken{
		Token:        "TODO-generate",
		IssuedReason: model.TokenReasonAnonymous,
	}
	now := time.Now()
	token.CreatedAt = now
	if err := s.DB.Create(token).Error; err != nil {
		return nil, err
	}
	return &pb.AuthResponse{Token: token.Token}, nil
}
