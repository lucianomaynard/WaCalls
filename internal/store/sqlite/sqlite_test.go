package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"wacalls/internal/voip/core"
)

func TestOpenConcurrencyConfig(t *testing.T) {
	bundle, err := Open(context.Background(), filepath.Join(t.TempDir(), "concurrency.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = bundle.Close() }()
	db := bundle.Sessions.(*sessionStore).db

	if got := db.Stats().MaxOpenConnections; got != 1 {
		t.Fatalf("expected pool capped to 1 connection, got %d", got)
	}

	var mode string
	if err := db.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if !strings.EqualFold(mode, "wal") {
		t.Fatalf("expected WAL journal mode, got %q", mode)
	}
}

func TestAuthStore(t *testing.T) {
	ctx := context.Background()
	b, err := Open(ctx, filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = b.Close() }()
	a := b.Auth

	if _, ok, _ := a.GetAdmin(ctx); ok {
		t.Fatal("no admin expected initially")
	}
	if err := a.CreateAdmin(ctx, "root", "hash1"); err != nil {
		t.Fatal(err)
	}
	cred, ok, err := a.GetAdmin(ctx)
	if err != nil || !ok || cred.Username != "root" || cred.PasswordHash != "hash1" {
		t.Fatalf("get admin: %+v ok=%v err=%v", cred, ok, err)
	}
	if err := a.SetAdminPassword(ctx, "hash2"); err != nil {
		t.Fatal(err)
	}
	if cred, _, _ := a.GetAdmin(ctx); cred.PasswordHash != "hash2" {
		t.Fatalf("password not updated: %q", cred.PasswordHash)
	}

	if err := a.CreateSession(ctx, "tokA", 1000); err != nil {
		t.Fatal(err)
	}
	if err := a.CreateSession(ctx, "tokB", 1000); err != nil {
		t.Fatal(err)
	}
	if ok, _ := a.SessionValid(ctx, "tokA", 999); !ok {
		t.Fatal("tokA should be valid at now<exp")
	}
	if ok, _ := a.SessionValid(ctx, "tokA", 1000); ok {
		t.Fatal("tokA should be invalid at now==exp")
	}
	if err := a.DeleteSessionsExcept(ctx, "tokA"); err != nil {
		t.Fatal(err)
	}
	if ok, _ := a.SessionValid(ctx, "tokB", 999); ok {
		t.Fatal("tokB should be gone")
	}
	if ok, _ := a.SessionValid(ctx, "tokA", 999); !ok {
		t.Fatal("tokA should remain")
	}
	_ = a.CreateSession(ctx, "old", 10)
	if err := a.PurgeExpiredSessions(ctx, 999); err != nil {
		t.Fatal(err)
	}
	if ok, _ := a.SessionValid(ctx, "old", 5); ok {
		t.Fatal("expired purged even against past now")
	}
}

func TestContactPhotoStore(t *testing.T) {
	b, err := Open(context.Background(), filepath.Join(t.TempDir(), "photos.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = b.Close() }()
	ctx := context.Background()
	jid := "5511@s.whatsapp.net"
	p := core.ContactPhoto{SessionID: "s1", Jid: jid, URL: "u1", PictureID: "id1", FetchedAt: 10}
	if err := b.Photos.Upsert(ctx, p); err != nil {
		t.Fatal(err)
	}
	got, ok, err := b.Photos.Get(ctx, "s1", jid)
	if err != nil || !ok || got.URL != "u1" {
		t.Fatalf("get: %+v ok=%v err=%v", got, ok, err)
	}
	p.URL, p.PictureID = "u2", "id2"
	if err := b.Photos.Upsert(ctx, p); err != nil {
		t.Fatal(err)
	}
	got, _, _ = b.Photos.Get(ctx, "s1", jid)
	if got.URL != "u2" || got.PictureID != "id2" {
		t.Fatalf("conflict update failed: %+v", got)
	}
	if _, ok, _ := b.Photos.Get(ctx, "s1", "absent"); ok {
		t.Fatal("absent jid should not be found")
	}
	m, err := b.Photos.GetMany(ctx, "s1", []string{jid, "absent"})
	if err != nil || len(m) != 1 || m[jid].URL != "u2" {
		t.Fatalf("getmany: %+v err=%v", m, err)
	}
	if m2, err := b.Photos.GetMany(ctx, "s1", nil); err != nil || len(m2) != 0 {
		t.Fatalf("getmany empty: %+v err=%v", m2, err)
	}
}

// perfex_calls: a hora do atendimento sobrevive à gravação/leitura do histórico.
func TestCallRecordConnectedAt(t *testing.T) {
	ctx := context.Background()
	b, err := Open(ctx, filepath.Join(t.TempDir(), "calls.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = b.Close() }()
	owner := "op1"
	if err := b.Calls.Insert(ctx, core.CallRecord{CallID: "A1", SessionID: "s", Owner: &owner, Direction: "inbound", Peer: "5579@s.whatsapp.net", StartedAt: 100, EndedAt: 300, EndReason: "user_ended", ConnectedAt: 150}); err != nil {
		t.Fatal(err)
	}
	if err := b.Calls.Insert(ctx, core.CallRecord{CallID: "A2", SessionID: "s", Direction: "outbound", Peer: "5579@s.whatsapp.net", StartedAt: 400, EndedAt: 500, EndReason: "user_ended"}); err != nil {
		t.Fatal(err)
	}
	rows, err := b.Calls.List(ctx, "s", 10, core.HistoryCursor{})
	if err != nil || len(rows) != 2 {
		t.Fatalf("list: %v %+v", err, rows)
	}
	got := map[string]int64{rows[0].CallID: rows[0].ConnectedAt, rows[1].CallID: rows[1].ConnectedAt}
	if got["A1"] != 150 || got["A2"] != 0 {
		t.Fatalf("connected_at errado: %+v", got)
	}
}

// perfex_calls: o banco do fork (tabela sessions sem schema_migrations, tabelas extras do
// Chatwoot/storage) é aberto e migrado sem perder a sessão do WhatsApp.
func TestOpenForkDatabase(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "fork.db")
	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		`CREATE TABLE sessions (id TEXT PRIMARY KEY, name TEXT NOT NULL, jid TEXT)`,
		`INSERT INTO sessions VALUES ('037c1f8f', 'WhatsApp', '557991233799:13@s.whatsapp.net')`,
		`CREATE TABLE chatwoot_integration (session_id TEXT PRIMARY KEY, base_url TEXT NOT NULL)`,
		`CREATE TABLE storage_config (id INTEGER PRIMARY KEY CHECK (id = 1), provider TEXT NOT NULL DEFAULT 'minio')`,
	} {
		if _, err := raw.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	_ = raw.Close()

	b, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("open do banco do fork: %v", err)
	}
	defer func() { _ = b.Close() }()
	list, err := b.Sessions.List(ctx)
	if err != nil || len(list) != 1 || list[0].ID != "037c1f8f" {
		t.Fatalf("sessão do fork perdida: %v %+v", err, list)
	}
	if err := b.Calls.Insert(ctx, core.CallRecord{CallID: "X1", SessionID: "037c1f8f", Direction: "inbound", Peer: "p", StartedAt: 1, EndedAt: 2, ConnectedAt: 1}); err != nil {
		t.Fatalf("histórico no banco migrado: %v", err)
	}
}
