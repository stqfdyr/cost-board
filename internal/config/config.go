package config

import (
	"flag"
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	Host    string
	Port    int
	DataDir string
}

func Parse(args []string) (Config, string, []string, error) {
	fs := flag.NewFlagSet("cost-board", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Cost Board — a self-hosted subscription cost tracker.\n\n")
		fmt.Fprintf(fs.Output(), "Usage:\n")
		fmt.Fprintf(fs.Output(), "  cost-board [flags]              Start the server\n")
		fmt.Fprintf(fs.Output(), "  cost-board set-credentials      Set or change login credentials\n")
		fmt.Fprintf(fs.Output(), "  cost-board import <file.json>   Import items from a JSON file\n\n")
		fmt.Fprintf(fs.Output(), "Flags:\n")
		fs.PrintDefaults()
	}

	host := fs.String("host", envOr("HOST", "0.0.0.0"), "listen host")
	port := fs.Int("port", envOrInt("PORT", 8083), "listen port")
	dataDir := fs.String("data-dir", envOr("DATA_DIR", "./data"), "data directory (SQLite + backups + credentials)")

	if err := fs.Parse(args); err != nil {
		return Config{}, "", nil, err
	}

	subcommand := ""
	subArgs := []string{}
	rest := fs.Args()
	if len(rest) > 0 {
		subcommand = rest[0]
		subArgs = rest[1:]
	}

	return Config{
		Host:    *host,
		Port:    *port,
		DataDir: *dataDir,
	}, subcommand, subArgs, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv("COST_BOARD_" + key); v != "" {
		return v
	}
	return fallback
}

func envOrInt(key string, fallback int) int {
	if v := os.Getenv("COST_BOARD_" + key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}
