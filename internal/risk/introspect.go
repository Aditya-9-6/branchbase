package risk

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// SchemaIntrospector extracts schema metadata and foreign key relationships from a database.
type SchemaIntrospector interface {
	IntrospectTable(ctx context.Context, tableName string) (*TableMetadata, error)
	ComputeSchemaSignature(ctx context.Context, tableName string) (string, error)
}

// PostgresIntrospector implements SchemaIntrospector for PostgreSQL engines.
type PostgresIntrospector struct {
	db *sql.DB
}

// NewPostgresIntrospector creates an introspector using an active PostgreSQL database connection.
func NewPostgresIntrospector(db *sql.DB) *PostgresIntrospector {
	return &PostgresIntrospector{db: db}
}

// IntrospectTable fetches columns and foreign key constraints involving tableName.
func (p *PostgresIntrospector) IntrospectTable(ctx context.Context, tableName string) (*TableMetadata, error) {
	if p.db == nil {
		return &TableMetadata{TableName: tableName}, nil
	}

	cleanTable := strings.Trim(tableName, `"'`)

	// 1. Fetch column names
	colQuery := `
		SELECT column_name
		FROM information_schema.columns
		WHERE table_name = $1
		ORDER BY ordinal_position ASC;
	`
	rows, err := p.db.QueryContext(ctx, colQuery, cleanTable)
	if err != nil {
		return nil, fmt.Errorf("failed to query columns for table %q: %w", cleanTable, err)
	}
	defer rows.Close()

	var columns []string
	for rows.Next() {
		var col string
		if err := rows.Scan(&col); err != nil {
			return nil, err
		}
		columns = append(columns, col)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// 2. Fetch Inbound Foreign Keys (Other tables pointing to this table)
	inboundFKQuery := `
		SELECT 
			tc.constraint_name,
			tc.table_name AS from_table,
			kcu.column_name AS from_column,
			ccu.table_name AS to_table,
			ccu.column_name AS to_column
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu 
			ON tc.constraint_name = kcu.constraint_name 
			AND tc.table_schema = kcu.table_schema
		JOIN information_schema.constraint_column_usage ccu 
			ON ccu.constraint_name = tc.constraint_name 
			AND ccu.table_schema = tc.table_schema
		WHERE tc.constraint_type = 'FOREIGN KEY'
		  AND ccu.table_name = $1;
	`
	inboundRows, err := p.db.QueryContext(ctx, inboundFKQuery, cleanTable)
	if err != nil {
		return nil, fmt.Errorf("failed to query inbound foreign keys for table %q: %w", cleanTable, err)
	}
	defer inboundRows.Close()

	var inboundFKs []ForeignKeyRef
	for inboundRows.Next() {
		var fk ForeignKeyRef
		if err := inboundRows.Scan(&fk.ConstraintName, &fk.FromTable, &fk.FromColumn, &fk.ToTable, &fk.ToColumn); err != nil {
			return nil, err
		}
		inboundFKs = append(inboundFKs, fk)
	}
	if err := inboundRows.Err(); err != nil {
		return nil, err
	}

	// 3. Fetch Outbound Foreign Keys (This table pointing to other tables)
	outboundFKQuery := `
		SELECT 
			tc.constraint_name,
			tc.table_name AS from_table,
			kcu.column_name AS from_column,
			ccu.table_name AS to_table,
			ccu.column_name AS to_column
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu 
			ON tc.constraint_name = kcu.constraint_name 
			AND tc.table_schema = kcu.table_schema
		JOIN information_schema.constraint_column_usage ccu 
			ON ccu.constraint_name = tc.constraint_name 
			AND ccu.table_schema = tc.table_schema
		WHERE tc.constraint_type = 'FOREIGN KEY'
		  AND tc.table_name = $1;
	`
	outboundRows, err := p.db.QueryContext(ctx, outboundFKQuery, cleanTable)
	if err != nil {
		return nil, fmt.Errorf("failed to query outbound foreign keys for table %q: %w", cleanTable, err)
	}
	defer outboundRows.Close()

	var outboundFKs []ForeignKeyRef
	for outboundRows.Next() {
		var fk ForeignKeyRef
		if err := outboundRows.Scan(&fk.ConstraintName, &fk.FromTable, &fk.FromColumn, &fk.ToTable, &fk.ToColumn); err != nil {
			return nil, err
		}
		outboundFKs = append(outboundFKs, fk)
	}
	if err := outboundRows.Err(); err != nil {
		return nil, err
	}

	return &TableMetadata{
		TableName:   cleanTable,
		Columns:     columns,
		InboundFKs:  inboundFKs,
		OutboundFKs: outboundFKs,
	}, nil
}

// ComputeSchemaSignature returns a hash of the table's current column names and inbound foreign keys.
func (p *PostgresIntrospector) ComputeSchemaSignature(ctx context.Context, tableName string) (string, error) {
	meta, err := p.IntrospectTable(ctx, tableName)
	if err != nil {
		return "", err
	}

	return FormatSchemaSignature(meta), nil
}

// FormatSchemaSignature creates a deterministic SHA256 string from table metadata.
func FormatSchemaSignature(meta *TableMetadata) string {
	if meta == nil {
		return "empty_schema"
	}

	var parts []string
	parts = append(parts, "table:"+meta.TableName)

	cols := append([]string(nil), meta.Columns...)
	sort.Strings(cols)
	parts = append(parts, "cols:"+strings.Join(cols, ","))

	var fks []string
	for _, fk := range meta.InboundFKs {
		fks = append(fks, fmt.Sprintf("%s.%s->%s.%s", fk.FromTable, fk.FromColumn, fk.ToTable, fk.ToColumn))
	}
	sort.Strings(fks)
	parts = append(parts, "inbound_fks:"+strings.Join(fks, "|"))

	hasher := sha256.New()
	hasher.Write([]byte(strings.Join(parts, ";")))
	return hex.EncodeToString(hasher.Sum(nil))
}

// FormatPromptContext converts TableMetadata into a human/LLM-readable summary string.
func FormatPromptContext(meta *TableMetadata) string {
	if meta == nil || (len(meta.Columns) == 0 && len(meta.InboundFKs) == 0) {
		return "table not found in database or schema is offline"
	}

	colsSummary := "no columns"
	if len(meta.Columns) > 0 {
		colsSummary = strings.Join(meta.Columns, ", ")
	}

	fkSummary := "no inbound foreign keys"
	if len(meta.InboundFKs) > 0 {
		var refs []string
		for _, fk := range meta.InboundFKs {
			refs = append(refs, fmt.Sprintf("%s(%s)", fk.FromTable, fk.FromColumn))
		}
		fkSummary = fmt.Sprintf("%d inbound FK references: %s", len(meta.InboundFKs), strings.Join(refs, ", "))
	}

	return fmt.Sprintf("%s (%s) — %s", meta.TableName, colsSummary, fkSummary)
}

// MockIntrospector provides a mock implementation for tests or offline execution.
type MockIntrospector struct {
	Tables map[string]*TableMetadata
}

// NewMockIntrospector returns a MockIntrospector populated with optional test data.
func NewMockIntrospector() *MockIntrospector {
	return &MockIntrospector{
		Tables: make(map[string]*TableMetadata),
	}
}

func (m *MockIntrospector) IntrospectTable(ctx context.Context, tableName string) (*TableMetadata, error) {
	cleanTable := strings.Trim(tableName, `"'`)
	if meta, ok := m.Tables[cleanTable]; ok {
		return meta, nil
	}
	return &TableMetadata{TableName: cleanTable}, nil
}

func (m *MockIntrospector) ComputeSchemaSignature(ctx context.Context, tableName string) (string, error) {
	meta, err := m.IntrospectTable(ctx, tableName)
	if err != nil {
		return "", err
	}
	return FormatSchemaSignature(meta), nil
}
