package main

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// startRecordingJanitor remove periodicamente gravações locais mais antigas que
// `retention`. É um backstop para o disco não encher: com storage externo, o
// arquivo é apagado logo após o upload; o janitor cobre falhas/backlog e o caso
// de ainda não haver storage configurado.
func (s *server) startRecordingJanitor(retention time.Duration) {
	if s.recDir == "" || retention <= 0 {
		return
	}
	go func() {
		s.cleanupRecordings(retention) // uma passada já no start
		t := time.NewTicker(time.Hour)
		defer t.Stop()
		for range t.C {
			s.cleanupRecordings(retention)
		}
	}()
	s.log.Info("recording janitor ativo", "dir", s.recDir, "retention", retention.String())
}

func (s *server) cleanupRecordings(retention time.Duration) {
	entries, err := os.ReadDir(s.recDir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-retention)
	removed := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !isRecordingFile(name) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			if os.Remove(filepath.Join(s.recDir, name)) == nil {
				removed++
			}
		}
	}
	if removed > 0 {
		s.log.Info("recording janitor: gravações antigas removidas", "count", removed)
	}
}

func isRecordingFile(name string) bool {
	return strings.HasSuffix(name, ".wav") || strings.HasSuffix(name, ".opus") || strings.HasSuffix(name, ".mp3")
}
