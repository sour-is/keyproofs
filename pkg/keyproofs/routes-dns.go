package keyproofs

import (
	"context"
	"net"
	"net/http"
	"strings"

	"github.com/go-chi/chi"
)

type dnsApp struct {
	resolver *net.Resolver
}

func NewDNSApp(ctx context.Context) *dnsApp {
	return &dnsApp{resolver: net.DefaultResolver}
}
func (app *dnsApp) getDNS(w http.ResponseWriter, r *http.Request) {
	domain := chi.URLParam(r, "domain")

	res, err := app.resolver.LookupTXT(r.Context(), domain)
	if err != nil {
		writeText(w, 400, err.Error())
		return
	}

	writeText(w, 200, strings.Join(res, "\n"))
}
func (app *dnsApp) Routes(r *chi.Mux) {
	r.MethodFunc("GET", "/dns/{domain}", app.getDNS)
}
