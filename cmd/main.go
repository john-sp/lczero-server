package main

import (
	"log"
	"net"

	"github.com/leelachesszero/lczero-server/internal/config"
	"github.com/leelachesszero/lczero-server/internal/db"

	"github.com/leelachesszero/lczero-server/internal/server"

	pb "github.com/leelachesszero/lczero-server/api/v1"

	"google.golang.org/grpc"
)

func main() {
	// Load configuration (reuses existing config loader).
	config.LoadConfig()
	log.Println("Configuration loaded successfully.")

	// Open DB

	db.Init()

	lis, err := net.Listen("tcp", config.Config.WebServer.Address)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	s := grpc.NewServer()

	// Register services
	pb.RegisterAuthServiceServer(s, server.NewAuthService(db.GetDB()))
	pb.RegisterTaskServiceServer(s, server.NewTaskService(db.GetDB()))

	log.Printf("gRPC server listening at %v", lis.Addr())
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
