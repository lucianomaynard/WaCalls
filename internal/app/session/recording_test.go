package session

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRecordingPath(t *testing.T) {
	dir := "/data/recordings"
	cases := map[string]string{
		"00DA0DE0853008434A5A59511D03FE68": filepath.Join(dir, "00DA0DE0853008434A5A59511D03FE68.wav"),
		"../etc/passwd":                    "",
		"abc":                              "",
		"a/b/c/d/e/f":                      "",
	}
	for id, want := range cases {
		if got := RecordingPath(dir, id); got != want {
			t.Errorf("RecordingPath(%q) = %q, want %q", id, got, want)
		}
	}
	if RecordingPath("", "00DA0DE0853008434A5A") != "" {
		t.Error("gravação desligada deve devolver caminho vazio")
	}
}

func TestNilRecorderIsSafe(t *testing.T) {
	var r *Recorder
	r.WriteAgent([]float32{0.1, 0.2})
	r.WritePeer([]float32{0.1})
	r.Close()
}

func TestRecorderWritesValidWAV(t *testing.T) {
	path := filepath.Join(t.TempDir(), "CALL123456.wav")
	r, err := NewRecorder(path)
	if err != nil {
		t.Fatal(err)
	}
	r.Start()
	frame := make([]float32, 320)
	for i := range frame {
		frame[i] = 0.25
	}
	for i := 0; i < 10; i++ {
		r.WriteAgent(frame)
		r.WritePeer(frame)
	}
	time.Sleep(60 * time.Millisecond)
	r.Close()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(b[0:4]) != "RIFF" || string(b[8:12]) != "WAVE" {
		t.Fatalf("cabeçalho WAV inválido")
	}
	data := binary.LittleEndian.Uint32(b[40:44])
	if int(data) != len(b)-44 || data == 0 || data%2 != 0 {
		t.Fatalf("tamanho de dados %d não confere com o arquivo (%d bytes)", data, len(b))
	}
	if binary.LittleEndian.Uint32(b[24:28]) != 16000 {
		t.Fatalf("taxa de amostragem errada")
	}
}
