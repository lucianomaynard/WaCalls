package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// storageConfig é a configuração (única) de armazenamento externo das gravações.
// Compatível com qualquer provedor S3 (MinIO, AWS S3, servidor próprio).
type storageConfig struct {
	Provider  string `json:"provider"` // rótulo livre: "minio" | "s3" | ...
	Endpoint  string `json:"endpoint"` // host:port (sem esquema), ex.: minio.exemplo.com
	Bucket    string `json:"bucket"`
	Region    string `json:"region"`
	AccessKey string `json:"access_key"`
	SecretKey string `json:"secret_key,omitempty"`
	Prefix    string `json:"prefix"` // prefixo/pasta dentro do bucket
	UseSSL    bool   `json:"use_ssl"`
	Enabled   bool   `json:"enabled"`
	UpdatedAt string `json:"updated_at"`
}

type storageStore struct{ db *sql.DB }

func newStorageStore(ctx context.Context, db *sql.DB) (*storageStore, error) {
	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS storage_config (
		id         INTEGER PRIMARY KEY CHECK (id = 1),
		provider   TEXT NOT NULL DEFAULT 'minio',
		endpoint   TEXT NOT NULL DEFAULT '',
		bucket     TEXT NOT NULL DEFAULT '',
		region     TEXT NOT NULL DEFAULT '',
		access_key TEXT NOT NULL DEFAULT '',
		secret_key TEXT NOT NULL DEFAULT '',
		prefix     TEXT NOT NULL DEFAULT '',
		use_ssl    INTEGER NOT NULL DEFAULT 1,
		enabled    INTEGER NOT NULL DEFAULT 0,
		updated_at TEXT NOT NULL DEFAULT (datetime('now'))
	)`)
	if err != nil {
		return nil, err
	}
	return &storageStore{db: db}, nil
}

func (s *storageStore) get(ctx context.Context) (*storageConfig, error) {
	row := s.db.QueryRowContext(ctx, `SELECT provider, endpoint, bucket, region, access_key,
		secret_key, prefix, use_ssl, enabled, updated_at FROM storage_config WHERE id = 1`)
	var c storageConfig
	var useSSL, enabled int
	err := row.Scan(&c.Provider, &c.Endpoint, &c.Bucket, &c.Region, &c.AccessKey,
		&c.SecretKey, &c.Prefix, &useSSL, &enabled, &c.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.UseSSL = useSSL != 0
	c.Enabled = enabled != 0
	return &c, nil
}

// upsert grava a config. Se keepSecretIfEmpty e o secret vier vazio, preserva o atual.
func (s *storageStore) upsert(ctx context.Context, c *storageConfig, keepSecretIfEmpty bool) error {
	secret := c.SecretKey
	if keepSecretIfEmpty && strings.TrimSpace(secret) == "" {
		if existing, err := s.get(ctx); err == nil && existing != nil {
			secret = existing.SecretKey
		}
	}
	useSSL := 0
	if c.UseSSL {
		useSSL = 1
	}
	enabled := 0
	if c.Enabled {
		enabled = 1
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO storage_config (id, provider, endpoint, bucket, region, access_key, secret_key, prefix, use_ssl, enabled, updated_at)
		VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'))
		ON CONFLICT(id) DO UPDATE SET
			provider=excluded.provider, endpoint=excluded.endpoint, bucket=excluded.bucket,
			region=excluded.region, access_key=excluded.access_key, secret_key=excluded.secret_key,
			prefix=excluded.prefix, use_ssl=excluded.use_ssl, enabled=excluded.enabled,
			updated_at=datetime('now')`,
		c.Provider, c.Endpoint, c.Bucket, c.Region, c.AccessKey, secret, c.Prefix, useSSL, enabled)
	return err
}

func (s *storageStore) delete(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM storage_config WHERE id = 1`)
	return err
}

// ---- Cliente S3 e pipeline de upload -------------------------------------

func storageClient(cfg *storageConfig) (*minio.Client, error) {
	return minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
		Region: cfg.Region,
	})
}

// objectKey monta a chave do objeto: <prefix>/<callID>.opus
func objectKey(cfg *storageConfig, callID string) string {
	return path.Join(strings.Trim(cfg.Prefix, "/"), callID+".opus")
}

// onRecordingFinalized é chamado quando o WAV de uma chamada é fechado. Se houver
// storage externo habilitado, converte para Opus, sobe e apaga os arquivos locais.
// Roda em goroutine própria (chamado assim pelo Recorder), então pode bloquear.
func (s *server) onRecordingFinalized(wavPath string) {
	cfg, err := s.storage.get(context.Background())
	if err != nil || cfg == nil || !cfg.Enabled || strings.TrimSpace(cfg.Bucket) == "" {
		return // sem storage externo → fica local (janitor limpa)
	}
	callID := strings.TrimSuffix(filepath.Base(wavPath), ".wav")
	if err := s.encodeAndUpload(cfg, callID, wavPath); err != nil {
		s.log.Warn("recording upload falhou", "call", callID, "err", err)
		return
	}
	s.log.Info("gravação enviada ao storage", "call", callID, "bucket", cfg.Bucket, "key", objectKey(cfg, callID))
}

func (s *server) encodeAndUpload(cfg *storageConfig, callID, wavPath string) error {
	opusPath := strings.TrimSuffix(wavPath, ".wav") + ".opus"
	// Converte WAV -> Opus (voz, 24 kbps): ~10x menor.
	cmd := exec.Command("ffmpeg", "-y", "-nostdin", "-i", wavPath, "-c:a", "libopus", "-b:a", "24k", opusPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ffmpeg: %v: %s", err, strings.TrimSpace(string(out)))
	}
	defer os.Remove(opusPath)

	cli, err := storageClient(cfg)
	if err != nil {
		return err
	}
	ctxB := context.Background()
	// Garante o bucket (cria se não existir).
	if exists, err := cli.BucketExists(ctxB, cfg.Bucket); err == nil && !exists {
		if err := cli.MakeBucket(ctxB, cfg.Bucket, minio.MakeBucketOptions{Region: cfg.Region}); err != nil {
			return fmt.Errorf("make bucket: %w", err)
		}
	}
	f, err := os.Open(opusPath)
	if err != nil {
		return err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return err
	}
	ctx := context.Background()
	if _, err := cli.PutObject(ctx, cfg.Bucket, objectKey(cfg, callID), f, st.Size(),
		minio.PutObjectOptions{ContentType: "audio/ogg"}); err != nil {
		return err
	}
	// sucesso: apaga o WAV local (o .opus é apagado pelo defer)
	os.Remove(wavPath)
	return nil
}

// ---- Handlers HTTP (Painel) ----------------------------------------------

// GET /api/storage — NUNCA devolve o secret (só has_secret).
func (s *server) handleStorageGet(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.storage.get(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	resp := map[string]any{"configured": false, "enabled": false, "has_secret": false}
	if cfg != nil {
		resp["configured"] = true
		resp["enabled"] = cfg.Enabled
		resp["provider"] = cfg.Provider
		resp["endpoint"] = cfg.Endpoint
		resp["bucket"] = cfg.Bucket
		resp["region"] = cfg.Region
		resp["access_key"] = cfg.AccessKey
		resp["prefix"] = cfg.Prefix
		resp["use_ssl"] = cfg.UseSSL
		resp["has_secret"] = strings.TrimSpace(cfg.SecretKey) != ""
		resp["updated_at"] = cfg.UpdatedAt
	}
	writeJSON(w, http.StatusOK, resp)
}

// PUT /api/storage — salva. Secret vazio = mantém o atual.
func (s *server) handleStorageSave(w http.ResponseWriter, r *http.Request) {
	var body storageConfig
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "corpo inválido"})
		return
	}
	if strings.TrimSpace(body.Endpoint) == "" || strings.TrimSpace(body.Bucket) == "" ||
		strings.TrimSpace(body.AccessKey) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "endpoint, bucket e access key são obrigatórios"})
		return
	}
	if body.Provider == "" {
		body.Provider = "minio"
	}
	if err := s.storage.upsert(r.Context(), &body, true); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.handleStorageGet(w, r)
}

// DELETE /api/storage — remove a config (volta a gravar só local).
func (s *server) handleStorageDelete(w http.ResponseWriter, r *http.Request) {
	if err := s.storage.delete(r.Context()); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// POST /api/storage/test — testa a conexão (bucket existe / credenciais ok).
// Usa o secret salvo se o corpo vier sem secret.
func (s *server) handleStorageTest(w http.ResponseWriter, r *http.Request) {
	var body storageConfig
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "corpo inválido"})
		return
	}
	if strings.TrimSpace(body.SecretKey) == "" {
		if existing, _ := s.storage.get(r.Context()); existing != nil {
			body.SecretKey = existing.SecretKey
		}
	}
	cli, err := storageClient(&body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	exists, err := cli.BucketExists(r.Context(), body.Bucket)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "bucket_exists": exists})
}

// serveRecordingFromStorage tenta servir a gravação do storage externo (Opus).
// Retorna true se serviu; false se não há storage ou o objeto não existe.
func (s *server) serveRecordingFromStorage(w http.ResponseWriter, r *http.Request, callID string) bool {
	cfg, err := s.storage.get(r.Context())
	if err != nil || cfg == nil || !cfg.Enabled || strings.TrimSpace(cfg.Bucket) == "" {
		return false
	}
	cli, err := storageClient(cfg)
	if err != nil {
		return false
	}
	obj, err := cli.GetObject(r.Context(), cfg.Bucket, objectKey(cfg, callID), minio.GetObjectOptions{})
	if err != nil {
		return false
	}
	defer obj.Close()
	stat, err := obj.Stat()
	if err != nil {
		return false // objeto não existe
	}
	w.Header().Set("Content-Type", "audio/ogg")
	w.Header().Set("Content-Length", strconv.FormatInt(stat.Size, 10))
	_, _ = io.Copy(w, obj)
	return true
}
