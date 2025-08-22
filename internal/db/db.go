package db

import (
	"database/sql"
	"fmt"
	"log"

	"github.com/leelachesszero/lczero-server/internal/config"
	_ "github.com/lib/pq"
)

var db *sql.DB

// Init initializes database.
func Init() {
	connStr := fmt.Sprintf(
		"host=%s user=%s dbname=%s sslmode=disable password=%s",
		config.Config.Database.Host,
		config.Config.Database.User,
		config.Config.Database.Dbname,
		config.Config.Database.Password,
	)
	var err error
	db, err = sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal("Unable to connect to DB: ", err)
	}
	if err = db.Ping(); err != nil {
		log.Fatal("Unable to ping DB: ", err)
	}
	log.Println("Database connection successfully established.")
}

// GetDB returns current database object
func GetDB() *sql.DB {
	return db
}
