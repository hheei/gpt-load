package migrations

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

const ID0031 = "0031_policy_bindings"

type policyConfigText0031 string

func (policyConfigText0031) GormDBDataType(db *gorm.DB, _ *schema.Field) string {
	if strings.EqualFold(db.Dialector.Name(), "mysql") {
		return "longtext"
	}
	return "text"
}

type policyBinding0031 struct {
	ID            uint                 `gorm:"primaryKey;autoIncrement"`
	Scope         string               `gorm:"type:varchar(32);not null;uniqueIndex:idx_policy_bindings_target,priority:1"`
	GroupID       uint                 `gorm:"not null;uniqueIndex:idx_policy_bindings_target,priority:2"`
	CredentialID  uint                 `gorm:"not null;default:0;uniqueIndex:idx_policy_bindings_target,priority:3"`
	Revision      uint64               `gorm:"type:bigint;not null;default:1"`
	SchemaVersion int                  `gorm:"not null;default:1"`
	Config        policyConfigText0031 `gorm:"not null"`
	CreatedAtMS   int64                `gorm:"column:created_at_ms;not null;autoCreateTime:milli"`
	UpdatedAtMS   int64                `gorm:"column:updated_at_ms;not null;autoUpdateTime:milli"`
}

func (policyBinding0031) TableName() string { return "policy_bindings" }

func Up0031(db *gorm.DB) error {
	if err := ValidateRecoverable0031(db); err != nil {
		return err
	}
	if !db.Migrator().HasTable(&policyBinding0031{}) {
		if err := db.Migrator().CreateTable(&policyBinding0031{}); err != nil {
			return fmt.Errorf("create policy bindings table: %w", err)
		}
	}
	if !db.Migrator().HasIndex(&policyBinding0031{}, "idx_policy_bindings_target") {
		if err := db.Migrator().CreateIndex(&policyBinding0031{}, "idx_policy_bindings_target"); err != nil {
			return fmt.Errorf("create policy bindings target index: %w", err)
		}
	}
	return Validate0031(db)
}

func ValidateRecoverable0031(db *gorm.DB) error {
	switch strings.ToLower(db.Dialector.Name()) {
	case "mysql", "postgres", "sqlite":
	default:
		return fmt.Errorf("policy bindings: unsupported database driver %q", db.Dialector.Name())
	}
	if !db.Migrator().HasTable(&policyBinding0031{}) {
		return nil
	}
	columns, err := db.Migrator().ColumnTypes(&policyBinding0031{})
	if err != nil {
		return err
	}
	expected := map[string]struct{}{
		"id":             {},
		"scope":          {},
		"group_id":       {},
		"credential_id":  {},
		"revision":       {},
		"schema_version": {},
		"config":         {},
		"created_at_ms":  {},
		"updated_at_ms":  {},
	}
	for _, column := range columns {
		colName := strings.ToLower(column.Name())
		delete(expected, colName)
		if colName != "config" {
			continue
		}
		typeName := strings.ToLower(column.DatabaseTypeName())
		if strings.EqualFold(db.Dialector.Name(), "mysql") && (typeName == "text" || typeName == "tinytext") {
			return fmt.Errorf("policy bindings column config must be mediumtext or longtext in MySQL to accommodate 256KiB")
		}
		if !strings.Contains(typeName, "text") && !strings.Contains(typeName, "json") {
			return fmt.Errorf("policy bindings column config must be text")
		}
	}
	if len(expected) > 0 {
		return fmt.Errorf("policy bindings table is missing expected columns")
	}

	return validate0031TargetIndex(db, false)
}

func Validate0031(db *gorm.DB) error {
	if !db.Migrator().HasTable(&policyBinding0031{}) {
		return fmt.Errorf("policy bindings table is missing")
	}
	if err := ValidateRecoverable0031(db); err != nil {
		return err
	}
	return validate0031TargetIndex(db, true)
}

func validate0031TargetIndex(db *gorm.DB, requireExists bool) error {
	const indexName = "idx_policy_bindings_target"
	var query string
	var args []any
	switch db.Dialector.Name() {
	case "postgres":
		// 与 0017 一致，按索引位置读取；GORM 的 GetIndexes 不保证 PostgreSQL 列序。
		query = `SELECT COALESCE(attribute.attname, '') AS name,
			definition.indisunique AS is_unique,
			definition.indisvalid AND definition.indisready AS is_valid,
			definition.indpred IS NULL AND definition.indexprs IS NULL AS is_complete
			FROM pg_index AS definition
			JOIN pg_class AS table_relation ON table_relation.oid = definition.indrelid
			JOIN pg_namespace AS namespace ON namespace.oid = table_relation.relnamespace
			JOIN pg_class AS index_relation ON index_relation.oid = definition.indexrelid
			CROSS JOIN LATERAL unnest(definition.indkey) WITH ORDINALITY AS key(attnum, position)
			LEFT JOIN pg_attribute AS attribute ON attribute.attrelid = table_relation.oid
				AND attribute.attnum = key.attnum
			WHERE namespace.nspname = current_schema() AND table_relation.relname = 'policy_bindings'
				AND index_relation.relname = ? ORDER BY key.position`
		args = []any{indexName}
	case "mysql":
		query = `SELECT COALESCE(column_name, '') AS name, non_unique = 0 AS is_unique,
			1 AS is_valid, sub_part IS NULL AS is_complete FROM information_schema.statistics
			WHERE table_schema = DATABASE() AND table_name = 'policy_bindings' AND index_name = ?
			ORDER BY seq_in_index`
		args = []any{indexName}
	case "sqlite":
		query = `SELECT COALESCE(column_info.name, '') AS name,
			index_list."unique" <> 0 AS is_unique, 1 AS is_valid,
			index_list.partial = 0 AS is_complete
			FROM pragma_index_xinfo(?) AS column_info
			JOIN pragma_index_list('policy_bindings') AS index_list ON index_list.name = ?
			WHERE column_info.key = 1 ORDER BY column_info.seqno`
		args = []any{indexName, indexName}
	}
	var columns []struct {
		Name       string
		IsUnique   bool
		IsValid    bool
		IsComplete bool
	}
	if err := db.Raw(query, args...).Scan(&columns).Error; err != nil {
		return err
	}
	if len(columns) == 0 {
		if !requireExists {
			return nil
		}
		return fmt.Errorf("policy bindings index idx_policy_bindings_target is missing")
	}
	expected := []string{"scope", "group_id", "credential_id"}
	if len(columns) != len(expected) {
		return fmt.Errorf("incompatible policy bindings target index: expected three columns")
	}
	for i, column := range columns {
		if !column.IsUnique || !column.IsValid || !column.IsComplete || !strings.EqualFold(column.Name, expected[i]) {
			return fmt.Errorf("incompatible policy bindings target index: must be a valid full unique index on (scope, group_id, credential_id)")
		}
	}
	return nil
}
