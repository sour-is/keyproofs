package app_vcard

import (
	"context"
	"fmt"
	"net/http"
	"net/mail"

	"github.com/go-chi/chi"
	"gosrc.io/xmpp"
)

type app struct {
	conn *connection
}

func New(ctx context.Context, xmppConfig *xmpp.Config) (*app, error) {
	conn, err := NewXMPP(ctx, xmppConfig)
	if err != nil {
		return nil, err
	}

	return &app{conn: conn}, nil
}
func (app *app) Routes(r *chi.Mux) {
	r.MethodFunc("GET", "/vcard/{jid}", app.getVCard)
}
func (app *app) getVCard(w http.ResponseWriter, r *http.Request) {
	jid := chi.URLParam(r, "jid")
	if _, err := mail.ParseAddress(jid); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, err)

		return
	}

	vcard, err := app.conn.GetXMPPVCard(r.Context(), jid)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, err)

		return
	}

	w.Header().Set("Content-Type", "text/xml")
	w.WriteHeader(200)
	fmt.Fprint(w, vcard)
}
