package main

import (
	"context"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// phoneFromJID devolve o telefone (só dígitos) do outro lado da chamada.
//
// O WhatsApp passou a identificar quem liga pelo LID ("70098211606569@lid"), um identificador de
// privacidade sem o número. O par LID→telefone fica no store do whatsmeow (whatsmeow_lid_map) e é
// consultado por lookup. JIDs de telefone ("5579…@s.whatsapp.net") já trazem o número.
func phoneFromJID(jidStr string, lookup func(types.JID) (types.JID, error)) string {
	j, err := types.ParseJID(jidStr)
	if err != nil {
		return ""
	}
	j = j.ToNonAD()
	switch j.Server {
	case types.DefaultUserServer:
		return j.User
	case types.HiddenUserServer:
		if lookup == nil {
			return ""
		}
		pn, err := lookup(j)
		if err != nil || pn.IsEmpty() {
			return ""
		}
		return pn.ToNonAD().User
	}
	return ""
}

// peerPhone resolve o telefone usando o store de LIDs da sessão.
func (s *Session) peerPhone(jidStr string) string {
	if s.client == nil || s.client.Store == nil || s.client.Store.LIDs == nil {
		return phoneFromJID(jidStr, nil)
	}
	return phoneFromJID(jidStr, func(lid types.JID) (types.JID, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		return s.client.Store.LIDs.GetPNForLID(ctx, lid)
	})
}
