package main

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"cost-board/internal/api"
	"cost-board/internal/auth"
	"cost-board/internal/config"
	"cost-board/internal/store"
)

//go:embed all:web/dist
var webFS embed.FS

func main() {
	cfg, subcommand, subArgs, err := config.Parse(os.Args[1:])
	if err != nil {
		os.Exit(2)
	}

	dbPath := filepath.Join(cfg.DataDir, "cost-board.db")

	switch subcommand {
	case "set-credentials":
		if err := auth.SetCredentialsInteractive(cfg.DataDir); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		return

	case "import":
		if len(subArgs) < 1 {
			fmt.Fprintln(os.Stderr, "usage: cost-board import <file.json>")
			os.Exit(2)
		}
		s, err := store.New(dbPath, cfg.DataDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		defer s.Close()
		if err := s.ImportJSON(subArgs[0]); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("import complete")
		return

	case "", "serve":
		runServer(cfg, dbPath)

	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", subcommand)
		os.Exit(2)
	}
}

func runServer(cfg config.Config, dbPath string) {
	s, err := store.New(dbPath, cfg.DataDir)
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer s.Close()

	a, err := auth.New(cfg.DataDir)
	if err != nil {
		log.Fatalf("auth: %v", err)
	}

	if !a.HasCredentials() {
		log.Println("WARNING: no credentials set. Run 'cost-board set-credentials' first.")
	}

	var embedFS http.FileSystem
	if dist, err := fs.Sub(webFS, "web/dist"); err == nil {
		if _, err := fs.Stat(dist, "index.html"); err == nil {
			embedFS = http.FS(dist)
		}
	}
	srv := api.NewServer(s, a, embedFS)
	if err := srv.Start(cfg.Host, cfg.Port); err != nil {
		log.Fatalf("server: %v", err)
	}
}
