package db

import (
	"net/url"
	"strings"
	"testing"
)

// ── ParseDSN ─────────────────────────────────────────────────────────────────

func TestParseDSN_JDBCForm(t *testing.T) {
	dsn, err := ParseDSN("jdbc:postgresql://db-host:5432/mydb", "alice", "s3cr3t")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(dsn, "alice") {
		t.Errorf("expected username in DSN, got: %q", dsn)
	}
	if !strings.Contains(dsn, "s3cr3t") {
		t.Errorf("expected password in DSN, got: %q", dsn)
	}
	if strings.HasPrefix(dsn, "jdbc:") {
		t.Errorf("DSN must not retain jdbc: prefix, got: %q", dsn)
	}
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

// TestParseDSN_PostgresAlias verifies that the shorter "postgres://" scheme
// alias (without "ql") is also accepted.
func TestParseDSN_PostgresAlias(t *testing.T) {
	dsn, err := ParseDSN("postgres://db-host:5432/mydb", "carol", "pw")
	if err != nil {
		t.Fatalf("unexpected error for postgres:// alias: %v", err)
	}
	if !strings.Contains(dsn, "carol") {
		t.Errorf("expected username in DSN, got: %q", dsn)
	}
}

// TestParseDSN_NoPort verifies that a URL without an explicit port is valid.
func TestParseDSN_NoPort(t *testing.T) {
	dsn, err := ParseDSN("jdbc:postgresql://db-host/mydb", "u", "p")
	if err != nil {
		t.Fatalf("unexpected error for URL without port: %v", err)
	}
	if !strings.Contains(dsn, "db-host") {
		t.Errorf("expected host in DSN, got: %q", dsn)
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

// TestParseDSN_MultipleQueryParams verifies all query parameters survive the
// JDBC-to-Go conversion intact.
func TestParseDSN_MultipleQueryParams(t *testing.T) {
	dsn, err := ParseDSN("jdbc:postgresql://host/db?sslmode=require&connect_timeout=10", "u", "p")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(dsn, "sslmode=require") {
		t.Errorf("expected sslmode in DSN, got: %q", dsn)
	}
	if !strings.Contains(dsn, "connect_timeout=10") {
		t.Errorf("expected connect_timeout in DSN, got: %q", dsn)
	}
}

// TestParseDSN_SpecialCharsInPassword verifies that passwords containing
// URL-special characters are encoded and round-trip correctly.
func TestParseDSN_SpecialCharsInPassword(t *testing.T) {
	const specialPass = `p@$$w0rd!`
	dsn, err := ParseDSN("jdbc:postgresql://host:5432/db", "user", specialPass)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Re-parse the produced URL to verify the password survives the round-trip.
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("ParseDSN returned an unparseable URL %q: %v", dsn, err)
	}
	gotPass, _ := u.User.Password()
	if gotPass != specialPass {
		t.Errorf("password round-trip: got %q, want %q", gotPass, specialPass)
	}
}

// TestParseDSN_EmptyURL rejects an empty string — there is no scheme to accept.
func TestParseDSN_EmptyURL(t *testing.T) {
	_, err := ParseDSN("", "u", "p")
	if err == nil {
		t.Fatal("expected error for empty URL, got nil")
	}
}

func TestParseDSN_UnsupportedScheme(t *testing.T) {
	_, err := ParseDSN("mysql://host/db", "u", "p")
	if err == nil {
		t.Fatal("expected error for unsupported scheme, got nil")
	}
}

// TestParseDSN_JDBCWithUnsupportedDriver verifies that jdbc: URLs for
// non-PostgreSQL drivers (e.g. jdbc:mysql://) are rejected.
func TestParseDSN_JDBCWithUnsupportedDriver(t *testing.T) {
	_, err := ParseDSN("jdbc:mysql://host/db", "u", "p")
	if err == nil {
		t.Fatal("expected error for jdbc:mysql://, got nil")
	}
}

func TestParseDSN_InvalidURL(t *testing.T) {
	_, err := ParseDSN("jdbc:postgresql://host with spaces/db", "u", "p")
	if err == nil {
		t.Fatal("expected error for invalid URL, got nil")
	}
}

