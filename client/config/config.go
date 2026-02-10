package config

import (
	"log"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	ServerAddr string `yaml:"server_addr"`
	User       struct {
		Name        string `yaml:"name"`
		Phone       string `yaml:"phone"`
		ProjectName string `yaml:"project_name"`
		Remark      string `yaml:"remark"`
	} `yaml:"user"`
}

var GlobalConfig Config

func Load() {
	// Default
	GlobalConfig.ServerAddr = "120.27.217.221:7001"

	data, err := os.ReadFile("config.yaml")
	if err != nil {
		log.Println("config.yaml not found, using defaults")
		return
	}

	err = yaml.Unmarshal(data, &GlobalConfig)
	if err != nil {
		log.Printf("Failed to parse config.yaml: %v", err)
	}
}

func Save(name, phone, projectName, remark string) {
	GlobalConfig.User.Name = name
	GlobalConfig.User.Phone = phone
	GlobalConfig.User.ProjectName = projectName
	GlobalConfig.User.Remark = remark

	data, err := yaml.Marshal(&GlobalConfig)
	if err != nil {
		log.Printf("Failed to marshal config: %v", err)
		return
	}

	err = os.WriteFile("config.yaml", data, 0644)
	if err != nil {
		log.Printf("Failed to save config.yaml: %v", err)
	}
}
