package main

import (
	"context"
	"database/sql"
	"log/slog"
	"net"

	"github.com/pion/webrtc/v4"
	"go.mau.fi/whatsmeow/store/sqlstore"
	waLog "go.mau.fi/whatsmeow/util/log"
	_ "modernc.org/sqlite"
)

type server struct {
	broker    *Broker
	sessions  *SessionManager
	log       *slog.Logger
	staticDir string
	// webrtcAPI, quando não-nil, carrega o SettingEngine do pion configurado para
	// browsers REMOTOS (NAT1To1 com IP público + UDP mux numa porta fixa). Nil = LAN default.
	webrtcAPI *webrtc.API
	// chatwoot persiste a integração Chatwoot por sessão.
	chatwoot *chatwootStore
	// publicBaseURL é a base pública usada para montar a URL do webhook do Chatwoot
	// (o engine é interno; normalmente é a URL pública do gateway).
	publicBaseURL string
	// recDir é o diretório onde as gravações WAV são salvas ("" = gravação desligada).
	recDir string
}

// buildWebRTCAPI monta a *webrtc.API para navegadores remotos.
// - publicIP != "" faz o pion anunciar esse IP nos candidatos ICE (NAT 1:1).
// - udpMuxPort > 0 força toda a mídia por UMA porta UDP (fácil de liberar/rotear).
// Retorna (nil, nil) quando nada configurado → mantém o comportamento LAN original.
func buildWebRTCAPI(publicIP string, udpMuxPort int, log *slog.Logger) (*webrtc.API, error) {
	if publicIP == "" && udpMuxPort == 0 {
		return nil, nil
	}
	se := webrtc.SettingEngine{}
	if publicIP != "" {
		se.SetNAT1To1IPs([]string{publicIP}, webrtc.ICECandidateTypeHost)
	}
	if udpMuxPort > 0 {
		conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4zero, Port: udpMuxPort})
		if err != nil {
			return nil, err
		}
		se.SetICEUDPMux(webrtc.NewICEUDPMux(nil, conn))
		log.Info("webrtc udp mux ready", "port", udpMuxPort, "publicIP", publicIP)
	}
	return webrtc.NewAPI(webrtc.WithSettingEngine(se)), nil
}

func openDB(dbPath string) (*sql.DB, error) {
	dsn := "file:" + dbPath + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	return db, nil
}

func newServer(ctx context.Context, dbPath, staticDir string, maxCalls int, publicIP string, udpMuxPort int, publicBaseURL, recordDir string, log *slog.Logger) (*server, error) {
	db, err := openDB(dbPath)
	if err != nil {
		return nil, err
	}
	container := sqlstore.NewWithDB(db, "sqlite3", waLog.Noop)
	if err := container.Upgrade(ctx); err != nil {
		return nil, err
	}
	store, err := newSessionStore(ctx, db)
	if err != nil {
		return nil, err
	}

	waLogger := waLog.Noop
	if log.Enabled(ctx, slog.LevelDebug) {
		waLogger = waLog.Stdout("WA", "INFO", true)
	}

	broker := NewBroker()
	mgr := newSessionManager(ctx, container, broker, store, waLogger, log, maxCalls)
	broker.SnapshotFn = mgr.snapshotEvents

	webrtcAPI, err := buildWebRTCAPI(publicIP, udpMuxPort, log)
	if err != nil {
		return nil, err
	}

	chatwoot, err := newChatwootStore(ctx, db)
	if err != nil {
		return nil, err
	}

	return &server{
		broker:        broker,
		sessions:      mgr,
		log:           log,
		staticDir:     staticDir,
		webrtcAPI:     webrtcAPI,
		chatwoot:      chatwoot,
		publicBaseURL: publicBaseURL,
		recDir:        recordDir,
	}, nil
}
