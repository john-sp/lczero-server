package config

import (
	"encoding/json"
	"os"
)

// Config is a Server config.
var Config struct {
	Database struct {
		Host     string
		User     string
		Dbname   string
		Password string
	}
	Clients struct {
		MinClientVersion  uint64
		NextClientVersion uint64
		MinEngineVersion  string
		NextEngineVersion string
	}
	URLs struct {
		OnNewNetwork    []string
		NetworkLocation string
	}
	Matches struct {
		Games      int
		Parameters []any
		Threshold  float64
	}
	WebServer struct {
		Address string
	}
}

func LoadConfig() {
	content, err := os.ReadFile("serverconfig.json")
	if err != nil {
		panic(err)
	}
	err = json.Unmarshal(content, &Config)
	if err != nil {
		panic(err)
	}
}
