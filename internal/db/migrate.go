package db

import (
	"fmt"
	"time"

	"github.com/jamesonstone/ding/internal/invoice"
	"gorm.io/gorm"
)

// Migration records an applied schema migration. The presence of a row marks a
// version as done, which makes RunMigrations safe to re-run.
type Migration struct {
	Version   int    `gorm:"primaryKey"`
	Name      string `gorm:"column:name"`
	AppliedAt time.Time
}

// TableName pins the GORM table name.
func (Migration) TableName() string { return "migrations" }

// migrationStep is one ordered, idempotent schema change.
type migrationStep struct {
	version int
	name    string
	fn      func(*gorm.DB) error
}

// steps is the ordered migration list. The customers/invoices schema (including
// the idempotency_key column and the §15 indexes) is created via AutoMigrate
// from the struct tags in package invoice.
var steps = []migrationStep{
	{
		version: 1,
		name:    "create_customers_invoices",
		fn: func(d *gorm.DB) error {
			return d.AutoMigrate(&invoice.Customer{}, &invoice.Invoice{})
		},
	},
}

// RunMigrations applies any pending migrations in order. It is idempotent: a
// second run with no new steps is a no-op and returns nil.
func RunMigrations(gdb *gorm.DB) error {
	if err := gdb.AutoMigrate(&Migration{}); err != nil {
		return fmt.Errorf("migrate: tracking table: %w", err)
	}
	for _, m := range steps {
		var existing Migration
		if err := gdb.Where("version = ?", m.version).First(&existing).Error; err == nil {
			continue // already applied
		}
		if err := m.fn(gdb); err != nil {
			return fmt.Errorf("migrate: %d (%s): %w", m.version, m.name, err)
		}
		rec := Migration{Version: m.version, Name: m.name, AppliedAt: time.Now().UTC()}
		if err := gdb.Create(&rec).Error; err != nil {
			return fmt.Errorf("migrate: record %d: %w", m.version, err)
		}
	}
	return nil
}
