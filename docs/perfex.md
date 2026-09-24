# Ramo `perfex` (perfex_calls)

Base: `develop` do [JotaDev66/WaCalls](https://github.com/JotaDev66/WaCalls) (d16a076). Acrescenta só o que o
módulo **perfex_calls** do Perfex CRM precisa:

| O quê | Onde |
|---|---|
| `connectedAt` no registro/histórico (unix ms; `null` = não atendida) — migração `connected_at` (SQLite e Postgres) | `internal/app/events/callregistry.go`, `internal/store/*`, `session.go` |
| Gravação WAV por chamada (atendente + cliente), limpeza por idade, `GET /api/recordings/{id}` (com Range) | `internal/app/session/recorder.go`, `recording.go`, `routes.go` |
| `GET /api/sessions/{sid}/qr` — estado e QR atual para o Perfex desenhar o pareamento | `handlers_session.go` |

Configuração nova: `WACALLS_RECORD_DIR` (vazio = sem gravação) e `WACALLS_RECORD_RETENTION_HOURS` (padrão 168).

O que o ramo antigo (`main` do fork) fazia e aqui já vem do upstream: IP público/porta UDP única
(`WACALLS_PUBLIC_IP`, `WACALLS_WEBRTC_UDP_PORT`), telefone real no lugar do LID, troca de navegador na mesma
chamada (transferência) com janela de tolerância de 30 s. Ficaram de fora: integração Chatwoot e storage S3.
