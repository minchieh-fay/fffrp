package config

import (
	"log"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server struct {
		TcpPort   int `yaml:"tcp_port"`
		WebPort   int `yaml:"web_port"`
		PortStart int `yaml:"port_start"`
	} `yaml:"server"`
}

var GlobalConfig Config

func Load() {
	// Default values
	GlobalConfig.Server.TcpPort = 7001
	GlobalConfig.Server.WebPort = 8080
	GlobalConfig.Server.PortStart = 10000

	data, err := os.ReadFile("config.yaml")
	if err != nil {
		log.Println("config.yaml not found, using default values")
		return
	}

	err = yaml.Unmarshal(data, &GlobalConfig)
	if err != nil {
		log.Fatalf("Failed to parse config.yaml: %v", err)
	}
}
