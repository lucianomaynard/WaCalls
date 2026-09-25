package call

import (
	"context"
	"log/slog"
	"sync"
	"testing"

	"wacalls/internal/voip/core"

	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/types"
)

// ownSock is a paired session: it knows its own LID/PN and records every stanza,
// including the ones sent through Query (terminate/reject go that way).
type ownSock struct {
	recordSock
	lid, pn types.JID
	qmu     sync.Mutex
	queried []waBinary.Node
}

func (s *ownSock) OwnLID() types.JID { return s.lid }
func (s *ownSock) OwnPN() types.JID  { return s.pn }
func (s *ownSock) Query(ctx context.Context, node waBinary.Node) (*waBinary.Node, error) {
	s.qmu.Lock()
	s.queried = append(s.queried, node)
	s.qmu.Unlock()
	return nil, nil
}
func (s *ownSock) queryCount() int {
	s.qmu.Lock()
	defer s.qmu.Unlock()
	return len(s.queried)
}

const (
	ownUser    = "210110236360906"
	callerUser = "70098211606569"
)

func ringingInbound(t *testing.T) (*CallManager, *ownSock, chan *CallInfo) {
	t.Helper()
	sock := &ownSock{
		lid: types.NewJID(ownUser, types.HiddenUserServer),
		pn:  types.NewJID("5579996005548", types.DefaultUserServer),
	}
	m := NewCallManager(sock, slog.Default())
	m.relay = &fakeRelay{noConn: true}
	caller := types.NewJID(callerUser, types.HiddenUserServer).String()
	m.currentCall = NewIncomingCall("CALL1", caller, caller, "", core.CallMediaTypeAudio)
	ended := make(chan *CallInfo, 1)
	m.OnEnded = func(c *CallInfo) { ended <- c }
	t.Cleanup(m.cleanupMedia)
	return m, sock, ended
}

func TestInboundAnsweredOnOwnPhoneEndsQuietly(t *testing.T) {
	m, sock, ended := ringingInbound(t)
	phone := lidDevice(ownUser, 74)

	m.HandleCallAccept(context.Background(), acceptNode("CALL1", phone), phone)

	select {
	case c := <-ended:
		if c.StateData.EndReason != core.EndCallReasonAnsweredElsewhere {
			t.Fatalf("end reason = %s, want %s", c.StateData.EndReason, core.EndCallReasonAnsweredElsewhere)
		}
	default:
		t.Fatal("call answered on the phone must end the engine's ringing")
	}
	// Nothing may go out: a terminate would hang up the call on the phone.
	if tags := sock.sentInnerTags(); len(tags) != 0 {
		t.Fatalf("no stanza may be sent, got %v", tags)
	}
	if n := sock.queryCount(); n != 0 {
		t.Fatalf("no stanza may be queried, got %d", n)
	}
	// The watchdog must not fire a timeout (and its terminate) later on.
	if err := m.EndCall(context.Background(), core.EndCallReasonTimeout); err != nil {
		t.Fatalf("end call: %v", err)
	}
	if n := sock.queryCount(); n != 0 {
		t.Fatalf("ended call must not send a terminate, got %d", n)
	}
}

func TestInboundAnsweredOnOwnPhoneByPN(t *testing.T) {
	m, _, _ := ringingInbound(t)
	phone := types.NewJID("5579996005548", types.DefaultUserServer)
	phone.Device = 3

	m.HandleCallAccept(context.Background(), acceptNode("CALL1", phone), phone)
	if s, r := stateOf(m); s != core.CallStateEnded || r != core.EndCallReasonAnsweredElsewhere {
		t.Fatalf("state = %s/%s, want ended/answered_elsewhere", s, r)
	}
}

func TestInboundAcceptFromOtherAccountKeepsRinging(t *testing.T) {
	m, _, ended := ringingInbound(t)
	stranger := lidDevice("99999999999999", 0)

	m.HandleCallAccept(context.Background(), acceptNode("CALL1", stranger), stranger)

	select {
	case <-ended:
		t.Fatal("an accept from another account must not end the call as answered elsewhere")
	default:
	}
	if s, _ := stateOf(m); s == core.CallStateEnded {
		t.Fatal("call must not be ended")
	}
}

func TestOwnPhoneAcceptForOtherCallIsIgnored(t *testing.T) {
	m, _, _ := ringingInbound(t)
	phone := lidDevice(ownUser, 74)

	m.HandleCallAccept(context.Background(), acceptNode("OTHER", phone), phone)

	if s, _ := stateOf(m); s != core.CallStateIncomingRinging {
		t.Fatalf("state = %s, want still ringing", s)
	}
}
