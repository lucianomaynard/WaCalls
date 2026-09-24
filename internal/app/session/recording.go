package session

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Gravação das chamadas (perfex_calls): WAV por chamada em <recordDir>/<callID>.wav, criado no
// primeiro áudio (atendente ou cliente) e finalizado quando a chamada sai do registro.
// Desligada quando recordDir está vazio (WACALLS_RECORD_DIR).

var recordingIDRe = regexp.MustCompile(`^[A-Za-z0-9_-]{6,64}$`)

// RecordingPath devolve o arquivo da gravação de callID dentro de dir, ou "" se o id for inválido
// (evita "../" e afins na rota GET /api/recordings/{id}).
func RecordingPath(dir, callID string) string {
	if dir == "" || !recordingIDRe.MatchString(callID) {
		return ""
	}
	return filepath.Join(dir, callID+".wav")
}

func (s *Session) recorderFor(callID string) *Recorder {
	if s.mgr.recordDir == "" {
		return nil
	}
	s.recMu.Lock()
	defer s.recMu.Unlock()
	if r, ok := s.recs[callID]; ok {
		return r
	}
	path := RecordingPath(s.mgr.recordDir, callID)
	if path == "" {
		return nil
	}
	if _, live := s.calls.Get(callID); !live {
		return nil
	}
	r, err := NewRecorder(path)
	if err != nil {
		s.log.Warn("recording disabled for call", "call_id", callID, "err", err)
		s.recs[callID] = nil // não tenta de novo a cada frame
		return nil
	}
	r.Start()
	s.recs[callID] = r
	return r
}

func (s *Session) closeRecorder(callID string) {
	s.recMu.Lock()
	r := s.recs[callID]
	delete(s.recs, callID)
	s.recMu.Unlock()
	r.Close()
}

func (s *Session) closeAllRecorders() {
	s.recMu.Lock()
	recs := s.recs
	s.recs = map[string]*Recorder{}
	s.recMu.Unlock()
	for _, r := range recs {
		r.Close()
	}
}

// StartRecordingJanitor apaga gravações mais antigas que retention (backstop de disco), uma vez no
// início e depois a cada hora, até ctx acabar.
func StartRecordingJanitor(ctx context.Context, dir string, retention time.Duration, log *slog.Logger) {
	if dir == "" || retention <= 0 {
		return
	}
	go func() {
		cleanupRecordings(dir, retention, log)
		t := time.NewTicker(time.Hour)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				cleanupRecordings(dir, retention, log)
			}
		}
	}()
	log.Info("recording janitor active", "dir", dir, "retention", retention.String())
}

func cleanupRecordings(dir string, retention time.Duration, log *slog.Logger) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-retention)
	removed := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".wav") {
			continue
		}
		info, err := e.Info()
		if err == nil && info.ModTime().Before(cutoff) && os.Remove(filepath.Join(dir, e.Name())) == nil {
			removed++
		}
	}
	if removed > 0 {
		log.Info("recording janitor removed old recordings", "count", removed)
	}
}
