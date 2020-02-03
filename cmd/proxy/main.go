package main

import (
	"github.com/spf13/viper"
	"log"
	"strings"
)

func main() {
	log.Printf("etcd-cluster-controller upload/download Proxy service")

	// Setup defaults for expected configuration keys
	viper.SetDefault("google-credential-location", "/var/credentials/google.json")
	viper.SetDefault("api-port", "8080")

	// Allow configuration to be passed via environment variables
	viper.SetEnvKeyReplacer(strings.NewReplacer("_", "-"))
	viper.AutomaticEnv()

	// Attempt to load a config file for configuration too
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("/etc/etcd-backup-restore-proxy/")

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			log.Print("Configuration file not present.")
		} else {
			log.Fatal(err, "Error reading configuration file")
		}
	}
}
