package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"go.mau.fi/whatsmeow/types"
)

func TestPhoneFromJID(t *testing.T) {
	lookup := func(lid types.JID) (types.JID, error) {
		if lid.User == "70098211606569" {
			return types.NewJID("5579999446677", types.DefaultUserServer), nil
		}
		return types.EmptyJID, errors.New("not found")
	}
	cases := []struct {
		name, jid, want string
		lookup          func(types.JID) (types.JID, error)
	}{
		{"telefone", "5579999446677@s.whatsapp.net", "5579999446677", nil},
		{"telefone com device", "5579999446677:13@s.whatsapp.net", "5579999446677", nil},
		{"lid mapeado", "70098211606569@lid", "5579999446677", lookup},
		{"lid com device", "70098211606569:3@lid", "5579999446677", lookup},
		{"lid sem par", "111@lid", "", lookup},
		{"lid sem store", "70098211606569@lid", "", nil},
		{"grupo", "123-456@g.us", "", lookup},
		{"vazio", "", "", lookup},
	}
	for _, c := range cases {
		if got := phoneFromJID(c.jid, c.lookup); got != c.want {
			t.Errorf("%s: phoneFromJID(%q) = %q, want %q", c.name, c.jid, got, c.want)
		}
	}
}

func TestBrokerHistoryCap(t *testing.T) {
	b := NewBroker()
	for i := 0; i < maxHistory+5; i++ {
		id := fmt.Sprintf("C%d", i)
		b.upsertCall(CallRecord{SessionID: "s", CallID: id, Direction: "inbound", Status: StatusRinging})
		b.endCall(id, "user_ended")
	}
	rows := b.historyRows("", maxHistory+100)
	if len(rows) != maxHistory {
		t.Fatalf("history not capped: %d rows", len(rows))
	}
	if rows[0].CallID != fmt.Sprintf("C%d", maxHistory+4) {
		t.Fatalf("newest row lost: %s", rows[0].CallID)
	}
}

func TestBrokerKeepsConnectedAtAndPhone(t *testing.T) {
	b := NewBroker()
	at := int64(1790217239041)
	b.upsertCall(CallRecord{SessionID: "s", CallID: "X1", Direction: "inbound", Peer: "1@lid", PeerPhone: "5579999446677", ConnectedAt: &at, Status: StatusConnected})
	b.endCall("X1", "user_ended")
	rows := b.historyRows("s", 10)
	if len(rows) != 1 || rows[0].PeerPhone != "5579999446677" || rows[0].ConnectedAt == nil || *rows[0].ConnectedAt != at {
		t.Fatalf("history lost peerPhone/connectedAt: %+v", rows)
	}
}

func TestConnectedAtAlwaysSerialized(t *testing.T) {
	b, err := json.Marshal(CallRecord{CallID: "X", Direction: "outbound", Status: StatusEnded})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"connectedAt":null`) {
		t.Fatalf("connectedAt must be present as null when not answered: %s", b)
	}
}
