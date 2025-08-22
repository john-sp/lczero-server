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

	"database/sql"

	"github.com/leelachesszero/lczero-server/internal/db/queries"
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
	DB *sql.DB
}

// generateUniqueToken generates a unique token with the prefix "lc0-" and 64 random characters.
// It checks the DB to ensure the token does not already exist.
func generateUniqueToken(db *sql.DB) (string, error) {
	const (
		prefix      = "lc0-"
		tokenLen    = 64
		maxAttempts = 10
	)

	for i := 0; i < maxAttempts; i++ {
		raw := make([]byte, tokenLen/2) // 32 bytes = 64 hex chars
		_, err := rand.Read(raw)
		if err != nil {
			return "", err
		}
		token := prefix + hex.EncodeToString(raw)

		var count int
		err = db.QueryRow(`SELECT COUNT(1) FROM auth_tokens WHERE token = $1`, token).Scan(&count)
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
func NewAuthService(dbConn *sql.DB) *AuthServiceImpl {
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
	row := s.DB.QueryRow(queries.SelectUserByUsername, req.Username)
	err := row.Scan(&user.ID, &user.Username, &user.Password, &user.AssignedTrainingRunID, &user.CreatedAt, &user.UpdatedAt, &user.DeletedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
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
	var tokenID uint
	err = s.DB.QueryRow(
		queries.InsertAuthToken,
		token.Token,
		token.IssuedReason,
		token.CreatedAt,
		user.ID,
	).Scan(&tokenID)
	if err != nil {
		return nil, status.Error(codes.Internal, "Failed to insert token")
	}
	token.ID = tokenID
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
	var tokenID uint
	err = s.DB.QueryRow(
		queries.InsertAuthToken,
		token.Token,
		token.IssuedReason,
		token.CreatedAt,
		nil,
	).Scan(&tokenID)
	if err != nil {
		return nil, status.Error(codes.Internal, "Failed to insert token")
	}
	token.ID = tokenID
	return &pb.AuthResponse{Token: token.Token}, nil
}
