package main

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Recorder grava uma chamada como WAV mono 16 kHz, misturando (somando) as duas
// direções: áudio do agente (browser) + áudio do lead (peer).
//
// A ingestão (WriteAgent/WritePeer) é NÃO-BLOQUEANTE: só copia amostras para um
// buffer sob mutex. Uma goroutine própria, com ticker de 20 ms, mistura e escreve
// no arquivo — assim o caminho de mídia da chamada nunca é bloqueado por I/O de disco.
type Recorder struct {
	mu       sync.Mutex
	agent    []int16
	peer     []int16
	f        *os.File
	path     string
	samples  int
	done     chan struct{}
	finished chan struct{}
	closed   bool
	// onDone é chamado (em goroutine) quando o WAV é finalizado, com o caminho do
	// arquivo. Usado para o upload ao storage externo.
	onDone func(path string)
}

const (
	recSampleRate  = 16000
	recFrame       = 320             // 20 ms @ 16 kHz
	recMaxBacklog  = recSampleRate * 5 // limita 5 s por direção (evita crescer sem fim)
)

// NewRecorder cria o arquivo WAV com um header placeholder (corrigido no Close).
func NewRecorder(path string) (*Recorder, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	if _, err := f.Write(wavHeader(0)); err != nil {
		f.Close()
		return nil, err
	}
	return &Recorder{f: f, path: path, done: make(chan struct{}), finished: make(chan struct{})}, nil
}

func (r *Recorder) Start() { go r.loop() }

// WriteAgent recebe o PCM capturado do microfone do agente (browser).
func (r *Recorder) WriteAgent(pcm []float32) { r.ingest(&r.agent, pcm) }

// WritePeer recebe o PCM decodificado do lado do lead (peer).
func (r *Recorder) WritePeer(pcm []float32) { r.ingest(&r.peer, pcm) }

func (r *Recorder) ingest(dst *[]int16, pcm []float32) {
	if r == nil || len(pcm) == 0 {
		return
	}
	r.mu.Lock()
	for _, s := range pcm {
		if s > 1 {
			s = 1
		} else if s < -1 {
			s = -1
		}
		*dst = append(*dst, int16(s*32767))
	}
	if len(*dst) > recMaxBacklog {
		*dst = append((*dst)[:0], (*dst)[len(*dst)-recMaxBacklog:]...)
	}
	r.mu.Unlock()
}

func (r *Recorder) loop() {
	defer close(r.finished)
	t := time.NewTicker(20 * time.Millisecond)
	defer t.Stop()
	buf := make([]byte, recFrame*2)
	for {
		select {
		case <-r.done:
			r.drainRemainder(buf)
			return
		case <-t.C:
			r.mixFrame(buf)
		}
	}
}

// mixFrame mistura um frame (20 ms) das duas direções e grava no WAV. Se uma
// direção não tem áudio suficiente, o restante do frame vira silêncio (mantém o
// alinhamento temporal / duração real).
func (r *Recorder) mixFrame(buf []byte) {
	r.mu.Lock()
	for i := 0; i < recFrame; i++ {
		var a, p int32
		if i < len(r.agent) {
			a = int32(r.agent[i])
		}
		if i < len(r.peer) {
			p = int32(r.peer[i])
		}
		v := a + p
		if v > 32767 {
			v = 32767
		} else if v < -32768 {
			v = -32768
		}
		binary.LittleEndian.PutUint16(buf[i*2:], uint16(int16(v)))
	}
	r.agent = advance(r.agent, recFrame)
	r.peer = advance(r.peer, recFrame)
	r.mu.Unlock()
	if _, err := r.f.Write(buf); err == nil {
		r.samples += recFrame
	}
}

// advance descarta os n primeiros samples (consumidos), preservando o resto.
func advance(b []int16, n int) []int16 {
	if len(b) >= n {
		return append(b[:0], b[n:]...)
	}
	return b[:0]
}

func (r *Recorder) drainRemainder(buf []byte) {
	for {
		r.mu.Lock()
		empty := len(r.agent) == 0 && len(r.peer) == 0
		r.mu.Unlock()
		if empty {
			return
		}
		r.mixFrame(buf)
	}
}

// Close encerra a gravação: para a goroutine, escreve o resto e corrige o header
// do WAV com o tamanho final. Idempotente.
func (r *Recorder) Close() {
	if r == nil {
		return
	}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}
	r.closed = true
	r.mu.Unlock()

	close(r.done)
	<-r.finished

	if _, err := r.f.Seek(0, 0); err == nil {
		_, _ = r.f.Write(wavHeader(r.samples))
	}
	r.f.Close()

	// Só finaliza (upload) se houve áudio de fato.
	if r.onDone != nil && r.samples > 0 {
		go r.onDone(r.path)
	}
}

// wavHeader monta o header PCM de 44 bytes para WAV mono 16-bit @ recSampleRate.
func wavHeader(numSamples int) []byte {
	const channels = 1
	const bits = 16
	byteRate := recSampleRate * channels * bits / 8
	blockAlign := channels * bits / 8
	dataSize := numSamples * blockAlign
	h := make([]byte, 44)
	copy(h[0:], "RIFF")
	binary.LittleEndian.PutUint32(h[4:], uint32(36+dataSize))
	copy(h[8:], "WAVE")
	copy(h[12:], "fmt ")
	binary.LittleEndian.PutUint32(h[16:], 16)
	binary.LittleEndian.PutUint16(h[20:], 1) // PCM
	binary.LittleEndian.PutUint16(h[22:], channels)
	binary.LittleEndian.PutUint32(h[24:], recSampleRate)
	binary.LittleEndian.PutUint32(h[28:], uint32(byteRate))
	binary.LittleEndian.PutUint16(h[32:], uint16(blockAlign))
	binary.LittleEndian.PutUint16(h[34:], bits)
	copy(h[36:], "data")
	binary.LittleEndian.PutUint32(h[40:], uint32(dataSize))
	return h
}
