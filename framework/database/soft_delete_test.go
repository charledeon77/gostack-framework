package database

import (
	"testing"
	"time"
)

type SoftDeleteUser struct {
	ID        int        `db:"id"`
	Name      string     `db:"name"`
	DeletedAt *time.Time `db:"deleted_at"`
}

func (s SoftDeleteUser) TableName() string {
	return "soft_delete_users"
}

func TestActiveRecord_SoftDeleteOperations(t *testing.T) {
	db := &mockDB{driver: "sqlite"}

	// 1. DeleteRecord sets deleted_at
	err := DeleteRecord[SoftDeleteUser](db, 1)
	if err != nil {
		t.Fatalf("DeleteRecord failed: %v", err)
	}

	// 2. Restore clears deleted_at
	err = Restore[SoftDeleteUser](db, 1)
	if err != nil {
		t.Fatalf("Restore failed: %v", err)
	}

	// 3. ForceDelete performs actual DELETE
	err = ForceDelete[SoftDeleteUser](db, 1)
	if err != nil {
		t.Fatalf("ForceDelete failed: %v", err)
	}
}
