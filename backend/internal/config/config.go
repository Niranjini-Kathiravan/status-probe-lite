package config

import "os"

type Config struct {
	Port    string
	DBPath  string
	Version string
}

func getEnv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func Load() *Config {
	return &Config{
		Port:    getEnv("PORT", "8080"),
		DBPath:  getEnv("DB_PATH", "./status.db"),
		Version: getEnv("VERSION", "v0.1"),
	}
}
