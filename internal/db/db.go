package db

import (
	"fmt"
	"log"

	"github.com/leelachesszero/lczero-server/internal/config"
	"github.com/leelachesszero/lczero-server/internal/models"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var db *gorm.DB
var err error

// Init initializes database.
func Init() {
	conn := fmt.Sprintf(
		"host=%s user=%s dbname=%s sslmode=disable password=%s",
		config.Config.Database.Host,
		config.Config.Database.User,
		config.Config.Database.Dbname,
		config.Config.Database.Password,
	)
	db, err = gorm.Open(postgres.Open(conn), &gorm.Config{})
	if err != nil {
		log.Fatal("Unable to connect to DB", err)
	}
	log.Println("Database connection successfully established.")

	log.Println("Running database migrations...")
	err = db.AutoMigrate(
		&models.User{},
		&models.Client{},
		&models.TrainingRun{},
		&models.Network{},
		&models.Match{},
		&models.MatchGame{},
		&models.TrainingGame{},
		&models.AuthToken{},
		&models.GrpcTask{})
	if err != nil {
		log.Fatalf("Failed to migrate database: %v", err)
	}
	log.Println("Database migration completed.")
}

// GetDB returns current database object
func GetDB() *gorm.DB {
	return db
}
