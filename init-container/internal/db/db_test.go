package db

import (
	"strings"
	"testing"
)

func TestParseDSN_JDBCForm(t *testing.T) {
	dsn, err := ParseDSN("jdbc:postgresql://db-host:5432/mydb", "alice", "s3cr3t")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Credentials must be embedded.
	if !strings.Contains(dsn, "alice") {
		t.Errorf("expected username in DSN, got: %q", dsn)
	}
	if !strings.Contains(dsn, "s3cr3t") {
		t.Errorf("expected password in DSN, got: %q", dsn)
	}
	// JDBC prefix must be stripped.
	if strings.HasPrefix(dsn, "jdbc:") {
		t.Errorf("DSN must not retain jdbc: prefix, got: %q", dsn)
	}
	// Host and DB must be preserved.
	if !strings.Contains(dsn, "db-host") {
		t.Errorf("expected host in DSN, got: %q", dsn)
	}
	if !strings.Contains(dsn, "mydb") {
		t.Errorf("expected dbname in DSN, got: %q", dsn)
	}
}

func TestParseDSN_PlainPostgresForm(t *testing.T) {
	dsn, err := ParseDSN("postgresql://db-host:5432/mydb", "bob", "pass")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(dsn, "bob") {
		t.Errorf("expected username in DSN, got: %q", dsn)
	}
}

func TestParseDSN_WithQueryParams(t *testing.T) {
	dsn, err := ParseDSN("jdbc:postgresql://db-host:5432/mydb?sslmode=require", "u", "p")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(dsn, "sslmode=require") {
		t.Errorf("expected query params preserved in DSN, got: %q", dsn)
	}
}

func TestParseDSN_UnsupportedScheme(t *testing.T) {
	_, err := ParseDSN("mysql://host/db", "u", "p")
	if err == nil {
		t.Fatal("expected error for unsupported scheme, got nil")
	}
}

func TestParseDSN_InvalidURL(t *testing.T) {
	_, err := ParseDSN("jdbc:postgresql://host with spaces/db", "u", "p")
	if err == nil {
		t.Fatal("expected error for invalid URL, got nil")
	}
}
