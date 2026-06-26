package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	_ "modernc.org/sqlite"
)

type Item struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Category     string  `json:"category"`
	Amount       float64 `json:"amount"`
	Currency     string  `json:"currency"`
	BillingMonths int     `json:"billingMonths"`
	Enabled      bool    `json:"enabled"`
	Note         string  `json:"note,omitempty"`
	Order        int     `json:"order"`
}

type Store struct {
	db        *sql.DB
	dataDir   string
	backupDir string
	maxBackups int
}

func New(dbPath, dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	backupDir := filepath.Join(dataDir, "backups")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return nil, fmt.Errorf("create backup dir: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	db.SetMaxOpenConns(1)

	s := &Store{
		db:         db,
		dataDir:    dataDir,
		backupDir:  backupDir,
		maxBackups: 20,
	}

	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}

	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS items (
			id             TEXT PRIMARY KEY,
			name           TEXT NOT NULL,
			category       TEXT NOT NULL,
			amount         REAL NOT NULL,
			currency       TEXT NOT NULL,
			billing_months INTEGER NOT NULL CHECK(billing_months IN (1,3,6,12)),
			enabled        INTEGER NOT NULL DEFAULT 1,
			note           TEXT,
			sort_order     INTEGER NOT NULL DEFAULT 0
		);
	`)
	return err
}

func (s *Store) GetAll() ([]Item, error) {
	rows, err := s.db.Query(`
		SELECT id, name, category, amount, currency, billing_months, enabled, note, sort_order
		FROM items
		ORDER BY sort_order ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []Item
	for rows.Next() {
		var it Item
		var enabled int
		var note sql.NullString
		if err := rows.Scan(&it.ID, &it.Name, &it.Category, &it.Amount, &it.Currency, &it.BillingMonths, &enabled, &note, &it.Order); err != nil {
			return nil, err
		}
		it.Enabled = enabled == 1
		if note.Valid {
			it.Note = note.String
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

func (s *Store) IsEmpty() (bool, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM items").Scan(&count)
	if err != nil {
		return false, err
	}
	return count == 0, nil
}

func (s *Store) ReplaceAll(items []Item) error {
	empty, _ := s.IsEmpty()
	if !empty {
		backup, err := s.exportJSON()
		if err == nil && backup != nil {
			s.writeBackup(backup)
		}
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec("DELETE FROM items"); err != nil {
		return fmt.Errorf("clear items: %w", err)
	}

	stmt, err := tx.Prepare(`
		INSERT INTO items (id, name, category, amount, currency, billing_months, enabled, note, sort_order)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, it := range items {
		enabled := 0
		if it.Enabled {
			enabled = 1
		}
		if _, err := stmt.Exec(it.ID, it.Name, it.Category, it.Amount, it.Currency, it.BillingMonths, enabled, it.Note, it.Order); err != nil {
			return fmt.Errorf("insert item %s: %w", it.ID, err)
		}
	}

	return tx.Commit()
}

func (s *Store) exportJSON() ([]byte, error) {
	items, err := s.GetAll()
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(items, "", "  ")
}

func (s *Store) writeBackup(data []byte) {
	stamp := time.Now().UTC().Format("2006-01-02T15-04-05.000Z")
	name := fmt.Sprintf("items-%s.json", stamp)
	path := filepath.Join(s.backupDir, name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return
	}
	s.rotateBackups()
}

func (s *Store) rotateBackups() {
	entries, err := os.ReadDir(s.backupDir)
	if err != nil {
		return
	}

	var names []string
	for _, e := range entries {
		if !e.IsDir() && len(e.Name()) > 6 && e.Name()[:6] == "items-" && e.Name()[len(e.Name())-5:] == ".json" {
			names = append(names, e.Name())
		}
	}

	sort.Sort(sort.Reverse(sort.StringSlice(names)))

	for i := s.maxBackups; i < len(names); i++ {
		os.Remove(filepath.Join(s.backupDir, names[i]))
	}
}

func (s *Store) ImportJSON(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}
	var items []Item
	if err := json.Unmarshal(data, &items); err != nil {
		return fmt.Errorf("parse json: %w", err)
	}
	return s.ReplaceAll(items)
}
