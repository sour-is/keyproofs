package keyproofs

import (
	"context"
	"fmt"
	"net/http"
	"net/mail"

	"github.com/go-chi/chi"
	zlog "github.com/rs/zerolog/log"
	"github.com/sour-is/keyproofs/pkg/config"
	"gosrc.io/xmpp"
)

type vcardApp struct {
	conn *connection
}

func NewVCardApp(ctx context.Context) (*vcardApp, error) {
	log := zlog.Ctx(ctx)

	var ok bool
	var xmppConfig *xmpp.Config
	if xmppConfig, ok = config.FromContext(ctx).Get("xmpp-config").(*xmpp.Config); !ok {
		log.Error().Msg("no xmpp-config")

		return nil, fmt.Errorf("no xmpp config")
	}

	conn, err := NewXMPP(ctx, xmppConfig)
	if err != nil {
		return nil, err
	}

	return &vcardApp{conn: conn}, nil
}
func (app *vcardApp) Routes(r *chi.Mux) {
	r.MethodFunc("GET", "/vcard/{jid}", app.getVCard)
}
func (app *vcardApp) getVCard(w http.ResponseWriter, r *http.Request) {
	jid := chi.URLParam(r, "jid")
	if _, err := mail.ParseAddress(jid); err != nil {
		fmt.Fprint(w, err)
		w.WriteHeader(400)
	}

	vcard, err := app.conn.GetXMPPVCard(r.Context(), jid)
	if err != nil {
		fmt.Fprint(w, err)
		w.WriteHeader(500)
	}

	w.Header().Set("Content-Type", "text/xml")
	w.WriteHeader(200)
	fmt.Fprint(w, vcard)
}
