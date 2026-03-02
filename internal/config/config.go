package config

import "github.com/spf13/viper"

type Config struct {
	Server struct {
		Port string
	}
	MySQL struct {
		DSN string
	}
	Redis struct {
		Addr     string
		Password string
		DB       int
	}
}

func Load() *Config {
	viper.SetConfigFile("configs/config.yaml")
	viper.ReadInConfig()

	var cfg Config
	viper.Unmarshal(&cfg)
	return &cfg
}
