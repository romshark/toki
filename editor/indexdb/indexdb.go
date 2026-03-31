// Package indexdb provides a SQLite-based index database for the Toki editor.
// It caches translation data so the editor can start quickly when .arb files
// haven't changed, and persists edits independently of the .arb files.
package indexdb

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	sqinn "github.com/cvilsmeier/sqinn-go/v2"
)

// DB wraps a sqinn-go SQLite connection for the index database.
type DB struct {
	sq   *sqinn.Sqinn
	path string
}

// Catalog represents a locale catalog stored in the index DB.
type Catalog struct {
	Locale          string
	Name            string
	IsDefault       bool
	MessagesCorrupt int
}

// TIK represents a translation key stored in the index DB.
type TIK struct {
	ID          string
	Raw         string
	Description string
}

// Message represents a single ICU message stored in the index DB.
type Message struct {
	TIKID              string
	Locale             string
	ICUMessage         string
	OriginalICUMessage string
	IsReadOnly         bool
}

// Open opens (or creates) the index database at the given path.
func Open(dbPath string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("creating index db directory: %w", err)
	}

	sq, err := sqinn.Launch(sqinn.Options{Db: dbPath})
	if err != nil {
		return nil, fmt.Errorf("launching sqinn: %w", err)
	}

	if err := createSchema(sq); err != nil {
		_ = sq.Close()
		return nil, err
	}

	return &DB{sq: sq, path: dbPath}, nil
}

func createSchema(sq *sqinn.Sqinn) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS meta (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS catalog (
			locale TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			is_default INTEGER NOT NULL DEFAULT 0,
			messages_corrupt INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS tik (
			id TEXT PRIMARY KEY,
			raw TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS message (
			tik_id TEXT NOT NULL,
			locale TEXT NOT NULL,
			icu_message TEXT NOT NULL DEFAULT '',
			original_icu_message TEXT NOT NULL DEFAULT '',
			is_read_only INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (tik_id, locale)
		)`,
	}
	for _, stmt := range stmts {
		if err := sq.ExecSql(stmt); err != nil {
			return fmt.Errorf("creating schema: %w", err)
		}
	}
	// Migrations for existing databases.
	migrations := []string{
		`ALTER TABLE catalog ADD COLUMN messages_corrupt INTEGER NOT NULL DEFAULT 0`,
	}
	for _, m := range migrations {
		_ = sq.ExecSql(m) // Ignore "duplicate column" errors.
	}
	return nil
}

// SchemaVersion is incremented when the index DB schema or detection logic
// changes in a way that requires a full rebuild of the cached data.
const SchemaVersion = "2"

// GetSchemaVersion returns the stored schema version, or "" if none.
func (db *DB) GetSchemaVersion() string {
	rows, err := db.sq.QueryRows(
		"SELECT value FROM meta WHERE key = ?",
		[]sqinn.Value{sqinn.StringValue("schema_version")},
		[]byte{sqinn.ValString},
	)
	if err != nil || len(rows) == 0 {
		return ""
	}
	return rows[0][0].String
}

// SetSchemaVersion stores the schema version.
func (db *DB) SetSchemaVersion(version string) error {
	return db.sq.ExecParams(
		"INSERT OR REPLACE INTO meta (key, value) VALUES (?, ?)",
		1, 2,
		[]sqinn.Value{
			sqinn.StringValue("schema_version"),
			sqinn.StringValue(version),
		},
	)
}

// Close terminates the sqinn subprocess and releases resources.
func (db *DB) Close() error {
	return db.sq.Close()
}

// GetChecksum returns the stored ARB file checksum, or "" if none.
func (db *DB) GetChecksum() (string, error) {
	rows, err := db.sq.QueryRows(
		"SELECT value FROM meta WHERE key = ?",
		[]sqinn.Value{sqinn.StringValue("arb_checksum")},
		[]byte{sqinn.ValString},
	)
	if err != nil {
		return "", fmt.Errorf("querying checksum: %w", err)
	}
	if len(rows) == 0 {
		return "", nil
	}
	return rows[0][0].String, nil
}

// SetChecksum stores the ARB file checksum.
func (db *DB) SetChecksum(checksum string) error {
	return db.sq.ExecParams(
		"INSERT OR REPLACE INTO meta (key, value) VALUES (?, ?)",
		1, 2,
		[]sqinn.Value{
			sqinn.StringValue("arb_checksum"),
			sqinn.StringValue(checksum),
		},
	)
}

// Clear removes all data from the database (catalogs, TIKs, messages)
// but preserves the meta table.
func (db *DB) Clear() error {
	for _, table := range []string{"message", "tik", "catalog"} {
		if err := db.sq.ExecSql("DELETE FROM " + table); err != nil {
			return fmt.Errorf("clearing %s: %w", table, err)
		}
	}
	return nil
}

// InsertCatalog inserts a catalog into the database.
func (db *DB) InsertCatalog(c Catalog) error {
	isDefault := 0
	if c.IsDefault {
		isDefault = 1
	}
	return db.sq.ExecParams(
		"INSERT OR REPLACE INTO catalog (locale, name, is_default, messages_corrupt) VALUES (?, ?, ?, ?)",
		1, 4,
		[]sqinn.Value{
			sqinn.StringValue(c.Locale),
			sqinn.StringValue(c.Name),
			sqinn.Int32Value(isDefault),
			sqinn.Int32Value(c.MessagesCorrupt),
		},
	)
}

// InsertTIK inserts a TIK into the database.
func (db *DB) InsertTIK(t TIK) error {
	return db.sq.ExecParams(
		"INSERT OR REPLACE INTO tik (id, raw, description) VALUES (?, ?, ?)",
		1, 3,
		[]sqinn.Value{
			sqinn.StringValue(t.ID),
			sqinn.StringValue(t.Raw),
			sqinn.StringValue(t.Description),
		},
	)
}

// InsertMessage inserts a message into the database.
func (db *DB) InsertMessage(m Message) error {
	isReadOnly := 0
	if m.IsReadOnly {
		isReadOnly = 1
	}
	return db.sq.ExecParams(
		"INSERT OR REPLACE INTO message (tik_id, locale, icu_message, original_icu_message, is_read_only) VALUES (?, ?, ?, ?, ?)",
		1, 5,
		[]sqinn.Value{
			sqinn.StringValue(m.TIKID),
			sqinn.StringValue(m.Locale),
			sqinn.StringValue(m.ICUMessage),
			sqinn.StringValue(m.OriginalICUMessage),
			sqinn.Int32Value(isReadOnly),
		},
	)
}

// UpdateMessage updates only the icu_message for a given tik_id and locale.
func (db *DB) UpdateMessage(tikID, locale, icuMessage string) error {
	return db.sq.ExecParams(
		"UPDATE message SET icu_message = ? WHERE tik_id = ? AND locale = ?",
		1, 3,
		[]sqinn.Value{
			sqinn.StringValue(icuMessage),
			sqinn.StringValue(tikID),
			sqinn.StringValue(locale),
		},
	)
}

// CommitMessages sets original_icu_message = icu_message for all messages,
// marking them as committed (no longer changed).
func (db *DB) CommitMessages() error {
	return db.sq.ExecSql("UPDATE message SET original_icu_message = icu_message")
}

// LoadCatalogs returns all catalogs from the database.
func (db *DB) LoadCatalogs() ([]Catalog, error) {
	rows, err := db.sq.QueryRows(
		"SELECT locale, name, is_default, messages_corrupt FROM catalog ORDER BY is_default DESC, locale",
		nil,
		[]byte{sqinn.ValString, sqinn.ValString, sqinn.ValInt32, sqinn.ValInt32},
	)
	if err != nil {
		return nil, fmt.Errorf("loading catalogs: %w", err)
	}
	catalogs := make([]Catalog, len(rows))
	for i, row := range rows {
		catalogs[i] = Catalog{
			Locale:          row[0].String,
			Name:            row[1].String,
			IsDefault:       row[2].Int32 != 0,
			MessagesCorrupt: row[3].Int32,
		}
	}
	return catalogs, nil
}

// LoadTIKs returns all TIKs from the database, sorted by ID.
func (db *DB) LoadTIKs() ([]TIK, error) {
	rows, err := db.sq.QueryRows(
		"SELECT id, raw, description FROM tik ORDER BY id",
		nil,
		[]byte{sqinn.ValString, sqinn.ValString, sqinn.ValString},
	)
	if err != nil {
		return nil, fmt.Errorf("loading tiks: %w", err)
	}
	tiks := make([]TIK, len(rows))
	for i, row := range rows {
		tiks[i] = TIK{
			ID:          row[0].String,
			Raw:         row[1].String,
			Description: row[2].String,
		}
	}
	return tiks, nil
}

// LoadMessages returns all messages from the database.
func (db *DB) LoadMessages() ([]Message, error) {
	rows, err := db.sq.QueryRows(
		"SELECT tik_id, locale, icu_message, original_icu_message, is_read_only FROM message ORDER BY tik_id, locale",
		nil,
		[]byte{sqinn.ValString, sqinn.ValString, sqinn.ValString, sqinn.ValString, sqinn.ValInt32},
	)
	if err != nil {
		return nil, fmt.Errorf("loading messages: %w", err)
	}
	msgs := make([]Message, len(rows))
	for i, row := range rows {
		msgs[i] = Message{
			TIKID:              row[0].String,
			Locale:             row[1].String,
			ICUMessage:         row[2].String,
			OriginalICUMessage: row[3].String,
			IsReadOnly:         row[4].Int32 != 0,
		}
	}
	return msgs, nil
}

// ChangedMessages returns messages where icu_message differs from original.
func (db *DB) ChangedMessages() ([]Message, error) {
	rows, err := db.sq.QueryRows(
		"SELECT tik_id, locale, icu_message, original_icu_message, is_read_only FROM message WHERE icu_message != original_icu_message",
		nil,
		[]byte{sqinn.ValString, sqinn.ValString, sqinn.ValString, sqinn.ValString, sqinn.ValInt32},
	)
	if err != nil {
		return nil, fmt.Errorf("loading changed messages: %w", err)
	}
	msgs := make([]Message, len(rows))
	for i, row := range rows {
		msgs[i] = Message{
			TIKID:              row[0].String,
			Locale:             row[1].String,
			ICUMessage:         row[2].String,
			OriginalICUMessage: row[3].String,
			IsReadOnly:         row[4].Int32 != 0,
		}
	}
	return msgs, nil
}

// BeginTx starts a transaction.
func (db *DB) BeginTx() error {
	return db.sq.ExecSql("BEGIN IMMEDIATE")
}

// Commit commits the current transaction.
func (db *DB) Commit() error {
	return db.sq.ExecSql("COMMIT")
}

// Rollback rolls back the current transaction.
func (db *DB) Rollback() error {
	return db.sq.ExecSql("ROLLBACK")
}

// ComputeARBChecksum computes a SHA-256 checksum over all .arb files
// in the given directory. Files are sorted by name for determinism.
func ComputeARBChecksum(bundleDir string) (string, error) {
	entries, err := os.ReadDir(bundleDir)
	if err != nil {
		return "", fmt.Errorf("reading bundle dir: %w", err)
	}

	var arbFiles []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if filepath.Ext(e.Name()) == ".arb" {
			arbFiles = append(arbFiles, filepath.Join(bundleDir, e.Name()))
		}
	}
	sort.Strings(arbFiles)

	h := sha256.New()
	for _, path := range arbFiles {
		f, err := os.Open(path)
		if err != nil {
			return "", fmt.Errorf("opening %s: %w", path, err)
		}
		// Write filename as separator to avoid collisions.
		_, _ = io.WriteString(h, filepath.Base(path))
		_, _ = io.WriteString(h, "\x00")
		if _, err := io.Copy(h, f); err != nil {
			_ = f.Close()
			return "", fmt.Errorf("reading %s: %w", path, err)
		}
		_ = f.Close()
		_, _ = io.WriteString(h, "\x00")
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}
