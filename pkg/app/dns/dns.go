package app_dns

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/go-chi/chi"
)

type app struct {
	resolver *net.Resolver
}

func New(ctx context.Context) *app {
	return &app{resolver: net.DefaultResolver}
}
func (app *app) getDNS(w http.ResponseWriter, r *http.Request) {
	domain := chi.URLParam(r, "domain")

	w.Header().Set("Content-Type", "text/plain")

	res, err := app.resolver.LookupTXT(r.Context(), domain)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)

		fmt.Fprintln(w, err)
		return
	}

	fmt.Fprintln(w, strings.Join(res, "\n"))
}
func (app *app) Routes(r *chi.Mux) {
	r.MethodFunc("GET", "/dns/{domain}", app.getDNS)
}
