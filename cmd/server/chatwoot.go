package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

// chatwootConfig é a integração Chatwoot vinculada a UMA sessão do WaCalls.
type chatwootConfig struct {
	SessionID       string `json:"session_id"`
	BaseURL         string `json:"base_url"`
	AccountID       string `json:"account_id"`
	InboxID         string `json:"inbox_id"`
	AccessToken     string `json:"access_token,omitempty"`
	InboxIdentifier string `json:"inbox_identifier"`
	Enabled         bool   `json:"enabled"`
	UpdatedAt       string `json:"updated_at"`
}

type chatwootStore struct{ db *sql.DB }

func newChatwootStore(ctx context.Context, db *sql.DB) (*chatwootStore, error) {
	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS chatwoot_integration (
		session_id       TEXT PRIMARY KEY,
		base_url         TEXT NOT NULL,
		account_id       TEXT NOT NULL,
		inbox_id         TEXT NOT NULL,
		access_token     TEXT NOT NULL DEFAULT '',
		inbox_identifier TEXT NOT NULL DEFAULT '',
		enabled          INTEGER NOT NULL DEFAULT 1,
		updated_at       TEXT NOT NULL DEFAULT (datetime('now'))
	)`)
	if err != nil {
		return nil, err
	}
	return &chatwootStore{db: db}, nil
}

func (s *chatwootStore) get(ctx context.Context, sid string) (*chatwootConfig, error) {
	row := s.db.QueryRowContext(ctx, `SELECT session_id, base_url, account_id, inbox_id,
		access_token, inbox_identifier, enabled, updated_at
		FROM chatwoot_integration WHERE session_id = ?`, sid)
	var c chatwootConfig
	var enabled int
	err := row.Scan(&c.SessionID, &c.BaseURL, &c.AccountID, &c.InboxID,
		&c.AccessToken, &c.InboxIdentifier, &enabled, &c.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.Enabled = enabled != 0
	return &c, nil
}

// upsert grava a config. Se keepTokenIfEmpty e o token vier vazio, preserva o atual.
func (s *chatwootStore) upsert(ctx context.Context, c *chatwootConfig, keepTokenIfEmpty bool) error {
	token := c.AccessToken
	if keepTokenIfEmpty && strings.TrimSpace(token) == "" {
		if existing, err := s.get(ctx, c.SessionID); err == nil && existing != nil {
			token = existing.AccessToken
		}
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO chatwoot_integration
			(session_id, base_url, account_id, inbox_id, access_token, inbox_identifier, enabled, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, 1, datetime('now'))
		ON CONFLICT(session_id) DO UPDATE SET
			base_url=excluded.base_url, account_id=excluded.account_id, inbox_id=excluded.inbox_id,
			access_token=excluded.access_token, inbox_identifier=excluded.inbox_identifier,
			enabled=1, updated_at=datetime('now')`,
		c.SessionID, c.BaseURL, c.AccountID, c.InboxID, token, c.InboxIdentifier)
	return err
}

func (s *chatwootStore) delete(ctx context.Context, sid string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM chatwoot_integration WHERE session_id = ?`, sid)
	return err
}

// webhookURL monta a URL pública que o Chatwoot deve chamar. Usa -public-base-url
// se configurado (o engine é interno; a URL pública é a do gateway), senão cai
// para o host da requisição.
func (s *server) webhookURL(r *http.Request, sid string) string {
	base := s.publicBaseURL
	if base == "" {
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		base = scheme + "://" + r.Host
	}
	return strings.TrimRight(base, "/") + "/api/sessions/" + sid + "/chatwoot/webhook"
}

// GET /api/sessions/{sid}/chatwoot — NUNCA devolve o valor do token (só has_token).
func (s *server) handleChatwootGet(w http.ResponseWriter, r *http.Request) {
	sid := r.PathValue("sid")
	cfg, err := s.chatwoot.get(r.Context(), sid)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	resp := map[string]any{
		"connected": cfg != nil && cfg.Enabled,
		"webhook":   s.webhookURL(r, sid),
		"has_token": false,
	}
	if cfg != nil {
		resp["base_url"] = cfg.BaseURL
		resp["account_id"] = cfg.AccountID
		resp["inbox_id"] = cfg.InboxID
		resp["inbox_identifier"] = cfg.InboxIdentifier
		resp["has_token"] = strings.TrimSpace(cfg.AccessToken) != ""
		resp["updated_at"] = cfg.UpdatedAt
	}
	writeJSON(w, http.StatusOK, resp)
}

// PUT /api/sessions/{sid}/chatwoot — salva. Token vazio = mantém o atual.
func (s *server) handleChatwootSave(w http.ResponseWriter, r *http.Request) {
	sid := r.PathValue("sid")
	var body chatwootConfig
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "corpo inválido"})
		return
	}
	body.SessionID = sid
	if strings.TrimSpace(body.BaseURL) == "" ||
		strings.TrimSpace(body.AccountID) == "" ||
		strings.TrimSpace(body.InboxID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "URL do Chatwoot, Account ID e Inbox ID são obrigatórios",
		})
		return
	}
	if _, err := strconv.Atoi(strings.TrimSpace(body.InboxID)); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Inbox ID deve ser um número"})
		return
	}
	if err := s.chatwoot.upsert(r.Context(), &body, true); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.handleChatwootGet(w, r) // devolve o estado atualizado (sem token)
}

// DELETE /api/sessions/{sid}/chatwoot — desconecta a integração.
func (s *server) handleChatwootDelete(w http.ResponseWriter, r *http.Request) {
	sid := r.PathValue("sid")
	if err := s.chatwoot.delete(r.Context(), sid); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// POST /api/sessions/{sid}/chatwoot/webhook — recebe eventos do Chatwoot.
// TODO técnico: aqui entra a ponte Chatwoot→WaCalls (ex.: mensagens de saída do
// agente → WhatsApp). Por ora aceitamos e logamos, para a inbox do Chatwoot não
// receber erro ao validar/entregar o webhook.
func (s *server) handleChatwootWebhook(w http.ResponseWriter, r *http.Request) {
	sid := r.PathValue("sid")
	cfg, err := s.chatwoot.get(r.Context(), sid)
	if err != nil || cfg == nil || !cfg.Enabled {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "sessão sem integração Chatwoot"})
		return
	}
	var payload map[string]any
	_ = json.NewDecoder(r.Body).Decode(&payload)
	s.log.Info("chatwoot webhook received", "session", sid, "event", payload["event"])
	writeJSON(w, http.StatusOK, map[string]bool{"received": true})
}
