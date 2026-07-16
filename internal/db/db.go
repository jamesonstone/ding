// Package db owns the SQLite connection and all data access for ding. State is
// stored in a single SQLite file at DING_DB_PATH (local filesystem in dev, a
// persistent single-writer volume in deployment). There is no external database.
package db

import (
	"fmt"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Open opens (creating if needed) the SQLite database at path with foreign keys
// enabled and a busy timeout so concurrent readers do not fail immediately.
// SQLite is a single-writer store, so the connection pool is capped at one.
func Open(path string) (*gorm.DB, error) {
	if path == "" {
		return nil, fmt.Errorf("db: empty database path (set DING_DB_PATH)")
	}
	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)", path)
	gdb, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("db: open %q: %w", path, err)
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		return nil, fmt.Errorf("db: handle: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)
	return gdb, nil
}

// Close releases the underlying database handle.
func Close(gdb *gorm.DB) error {
	sqlDB, err := gdb.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
