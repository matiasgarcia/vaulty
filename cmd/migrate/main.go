package main

import (
	"fmt"
	"log"
	"os"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"

	"github.com/pci-vault/vault/config"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: migrate <up|down|version>\n")
		os.Exit(1)
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	m, err := migrate.New("file://migrations", cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("init migrate: %v", err)
	}
	defer m.Close()

	switch os.Args[1] {
	case "up":
		if err := m.Up(); err != nil && err != migrate.ErrNoChange {
			log.Fatalf("migrate up: %v", err)
		}
		fmt.Println("Migrations applied successfully")
	case "down":
		if err := m.Down(); err != nil && err != migrate.ErrNoChange {
			log.Fatalf("migrate down: %v", err)
		}
		fmt.Println("Migrations reverted successfully")
	case "version":
		v, dirty, err := m.Version()
		if err != nil {
			log.Fatalf("migrate version: %v", err)
		}
		fmt.Printf("Version: %d, Dirty: %v\n", v, dirty)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\nUsage: migrate <up|down|version>\n", os.Args[1])
		os.Exit(1)
	}
}
