package call

import (
	"log/slog"
	"testing"

	"wacalls/internal/voip/core"
)

// A gravação (internal/app/session/recording.go) só começa quando Answered() é true.
func TestAnsweredOnlyAfterAccept(t *testing.T) {
	m := NewCallManager(fakeSock{}, slog.Default())
	if m.Answered() {
		t.Fatal("no call must not count as answered")
	}

	m.currentCall = NewIncomingCall("in1", "peer@lid", "peer@lid", "", core.CallMediaTypeAudio)
	if m.Answered() {
		t.Fatal("ringing inbound call must not count as answered")
	}
	if err := m.currentCall.ApplyTransition(Transition{Type: TransitionLocalAccepted}); err != nil {
		t.Fatalf("accept: %v", err)
	}
	if !m.Answered() {
		t.Fatal("inbound call accepted by us must count as answered")
	}
	_ = m.currentCall.ApplyTransition(Transition{Type: TransitionTerminated, Reason: core.EndCallReasonUserEnded})
	if m.Answered() {
		t.Fatal("ended call must not count as answered")
	}

	m.currentCall = NewOutgoingCall("out1", "peer@lid", "me@lid", core.CallMediaTypeAudio)
	_ = m.currentCall.ApplyTransition(Transition{Type: TransitionOfferSent})
	if m.Answered() {
		t.Fatal("outgoing call still ringing must not count as answered")
	}
	if err := m.currentCall.ApplyTransition(Transition{Type: TransitionRemoteAccepted}); err != nil {
		t.Fatalf("remote accept: %v", err)
	}
	if !m.Answered() {
		t.Fatal("outgoing call accepted by the peer must count as answered")
	}
}

func TestAnsweredElsewhereIsNotAnswered(t *testing.T) {
	m := NewCallManager(fakeSock{}, slog.Default())
	m.currentCall = NewIncomingCall("in1", "peer@lid", "peer@lid", "", core.CallMediaTypeAudio)
	_ = m.currentCall.ApplyTransition(Transition{Type: TransitionTerminated, Reason: core.EndCallReasonAnsweredElsewhere})
	if m.Answered() {
		t.Fatal("call answered on the phone must not be recorded here")
	}
}
