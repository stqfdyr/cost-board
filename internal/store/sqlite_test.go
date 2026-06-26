package store

import (
	"os"
	"path/filepath"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	s, err := New(dbPath, dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestNewAndMigrate(t *testing.T) {
	s := newTestStore(t)
	if s == nil {
		t.Fatal("expected non-nil store")
	}
	if _, err := os.Stat(filepath.Join(s.dataDir, "backups")); err != nil {
		t.Fatalf("backups dir not created: %v", err)
	}
}

func TestIsEmptyOnNewStore(t *testing.T) {
	s := newTestStore(t)
	empty, err := s.IsEmpty()
	if err != nil {
		t.Fatalf("IsEmpty: %v", err)
	}
	if !empty {
		t.Fatal("new store should be empty")
	}
}

func TestReplaceAndGetAll(t *testing.T) {
	s := newTestStore(t)
	items := []Item{
		{ID: "a", Name: "A", Category: "cat1", Amount: 10, Currency: "USD", BillingMonths: 12, Enabled: true, Order: 0},
		{ID: "b", Name: "B", Category: "cat2", Amount: 20, Currency: "CNY", BillingMonths: 1, Enabled: false, Order: 1, Note: "test note"},
	}
	if err := s.ReplaceAll(items); err != nil {
		t.Fatalf("ReplaceAll: %v", err)
	}

	got, err := s.GetAll()
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 items, got %d", len(got))
	}
	if got[0].ID != "a" || got[1].ID != "b" {
		t.Fatalf("unexpected order: %s, %s", got[0].ID, got[1].ID)
	}
	if got[1].Note != "test note" {
		t.Fatalf("expected note \"test note\", got %q", got[1].Note)
	}
	if !got[0].Enabled {
		t.Fatal("item a should be enabled")
	}
	if got[1].Enabled {
		t.Fatal("item b should be disabled")
	}
}

func TestReplaceAllOverwrites(t *testing.T) {
	s := newTestStore(t)
	items1 := []Item{
		{ID: "a", Name: "A", Category: "c", Amount: 1, Currency: "CNY", BillingMonths: 12, Enabled: true, Order: 0},
		{ID: "b", Name: "B", Category: "c", Amount: 2, Currency: "CNY", BillingMonths: 12, Enabled: true, Order: 1},
	}
	s.ReplaceAll(items1)

	items2 := []Item{
		{ID: "c", Name: "C", Category: "c", Amount: 3, Currency: "CNY", BillingMonths: 12, Enabled: true, Order: 0},
	}
	s.ReplaceAll(items2)

	got, _ := s.GetAll()
	if len(got) != 1 {
		t.Fatalf("expected 1 item after replace, got %d", len(got))
	}
	if got[0].ID != "c" {
		t.Fatalf("expected id c, got %s", got[0].ID)
	}
}

func TestReplaceAllEmptySlice(t *testing.T) {
	s := newTestStore(t)
	s.ReplaceAll([]Item{
		{ID: "a", Name: "A", Category: "c", Amount: 1, Currency: "CNY", BillingMonths: 12, Enabled: true, Order: 0},
	})
	s.ReplaceAll([]Item{})

	empty, _ := s.IsEmpty()
	if !empty {
		t.Fatal("store should be empty after replacing with empty slice")
	}
}

func TestReplaceAllNilSlice(t *testing.T) {
	s := newTestStore(t)
	err := s.ReplaceAll(nil)
	if err != nil {
		t.Fatalf("ReplaceAll(nil): %v", err)
	}
	empty, _ := s.IsEmpty()
	if !empty {
		t.Fatal("store should be empty after replacing with nil")
	}
}

func TestGetAllEmptyReturnsNil(t *testing.T) {
	s := newTestStore(t)
	got, err := s.GetAll()
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil for empty store, got %v", got)
	}
}

func TestBillingMonthsConstraint(t *testing.T) {
	s := newTestStore(t)
	// BillingMonths values 1,3,6,12 should succeed
	for _, bm := range []int{1, 3, 6, 12} {
		err := s.ReplaceAll([]Item{
			{ID: "x", Name: "X", Category: "c", Amount: 1, Currency: "CNY", BillingMonths: bm, Enabled: true, Order: 0},
		})
		if err != nil {
			t.Errorf("BillingMonths=%d should succeed: %v", bm, err)
		}
	}
}

func TestImportJSON(t *testing.T) {
	s := newTestStore(t)
	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "items.json")
	data := `[
		{"id":"a","name":"A","category":"c","amount":10,"currency":"USD","billingMonths":12,"enabled":true,"order":0}
	]`
	if err := os.WriteFile(jsonPath, []byte(data), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	if err := s.ImportJSON(jsonPath); err != nil {
		t.Fatalf("ImportJSON: %v", err)
	}

	got, _ := s.GetAll()
	if len(got) != 1 {
		t.Fatalf("expected 1 item, got %d", len(got))
	}
	if got[0].ID != "a" || got[0].Amount != 10 {
		t.Fatalf("unexpected item: %+v", got[0])
	}
}

func TestImportJSONFileNotFound(t *testing.T) {
	s := newTestStore(t)
	err := s.ImportJSON("/nonexistent/path.json")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestImportJSONInvalidJSON(t *testing.T) {
	s := newTestStore(t)
	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "bad.json")
	os.WriteFile(jsonPath, []byte("not json"), 0644)

	err := s.ImportJSON(jsonPath)
	if err == nil {
		t.Fatal("expected error for invalid json")
	}
}

func TestBackupCreatedOnReplace(t *testing.T) {
	s := newTestStore(t)
	s.ReplaceAll([]Item{
		{ID: "a", Name: "A", Category: "c", Amount: 1, Currency: "CNY", BillingMonths: 12, Enabled: true, Order: 0},
	})
	s.ReplaceAll([]Item{
		{ID: "b", Name: "B", Category: "c", Amount: 2, Currency: "CNY", BillingMonths: 12, Enabled: true, Order: 0},
	})

	entries, err := os.ReadDir(s.backupDir)
	if err != nil {
		t.Fatalf("read backup dir: %v", err)
	}
	count := 0
	for _, e := range entries {
		if !e.IsDir() && len(e.Name()) > 6 && e.Name()[:6] == "items-" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected 1 backup, got %d", count)
	}
}

func TestBackupRotation(t *testing.T) {
	s := newTestStore(t)
	s.maxBackups = 3

	for i := 0; i < 5; i++ {
		s.ReplaceAll([]Item{
			{ID: string(rune('a' + i)), Name: "X", Category: "c", Amount: float64(i), Currency: "CNY", BillingMonths: 12, Enabled: true, Order: 0},
		})
	}

	entries, _ := os.ReadDir(s.backupDir)
	count := 0
	for _, e := range entries {
		if !e.IsDir() && len(e.Name()) > 6 && e.Name()[:6] == "items-" {
			count++
		}
	}
	if count != 3 {
		t.Fatalf("expected 3 backups after rotation, got %d", count)
	}
}
