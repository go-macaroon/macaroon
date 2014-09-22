package main

import (
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/rogpeppe/macaroon/bakery"
	"github.com/rogpeppe/macaroon/bakery/checkers"
	"github.com/rogpeppe/macaroon/httpbakery"
)

type targetServiceHandler struct {
	svc          *httpbakery.Service
	authEndpoint string
	endpoint     string
	mux          *http.ServeMux
}

// targetService implements a "target service", representing
// an arbitrary web service that wants to delegate authorization
// to third parties.
//
func targetService(endpoint, authEndpoint string) (http.Handler, error) {
	svc, err := httpbakery.NewService(httpbakery.NewServiceParams{
		Location: endpoint,
	})
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	srv := &targetServiceHandler{
		svc:          svc,
		authEndpoint: authEndpoint,
	}
	mux.HandleFunc("/gold/", srv.serveGold)
	mux.HandleFunc("/silver/", srv.serveSilver)
	return mux, nil
}

func (srv *targetServiceHandler) serveGold(w http.ResponseWriter, req *http.Request) {
	breq := srv.svc.NewRequest(req, srv.checkers(req, "gold"))
	if err := breq.Check(); err != nil {
		srv.writeError(w, "gold", err)
		return
	}
	fmt.Fprintf(w, "all is golden")
}

func (srv *targetServiceHandler) serveSilver(w http.ResponseWriter, req *http.Request) {
	breq := srv.svc.NewRequest(req, srv.checkers(req, "silver"))
	if err := breq.Check(); err != nil {
		srv.writeError(w, "silver", err)
		return
	}
	fmt.Fprintf(w, "every cloud has a silver lining")
}

// checkers implements the caveat checking for the service.
// Note how we add context-sensitive checkers
// (remote-host checks information from the HTTP request)
// to the standard checkers implemented by checkers.Std.
func (svc *targetServiceHandler) checkers(req *http.Request, operation string) bakery.FirstPartyChecker {
	m := checkers.Map{
		"remote-host": func(s string) error {
			// TODO(rog) do we want to distinguish between
			// the two kinds of errors below?
			_, host, err := checkers.ParseCaveat(s)
			if err != nil {
				return err
			}
			remoteHost, _, err := net.SplitHostPort(req.RemoteAddr)
			if err != nil {
				return fmt.Errorf("cannot parse request remote address")
			}
			if remoteHost != host {
				return fmt.Errorf("remote address mismatch (need %q, got %q)", host, remoteHost)
			}
			return nil
		},
		"operation": func(s string) error {
			_, op, err := checkers.ParseCaveat(s)
			if err != nil {
				return err
			}
			if op != operation {
				return fmt.Errorf("macaroon not valid for operation")
			}
			return nil
		},
	}
	return checkers.PushFirstPartyChecker(m, checkers.Std)
}

// writeError writes an error to w. If the error was generated because
// of a required macaroon that the client does not have, we mint a
// macaroon that, when discharged, will grant the client the
// right to execute the given operation.
//
// The logic in this function is crucial to the security of the service
// - it must determine for a given operation what caveats to attach.
func (srv *targetServiceHandler) writeError(w http.ResponseWriter, operation string, verr error) {
	fail := func(code int, msg string, args ...interface{}) {
		if code == http.StatusInternalServerError {
			msg = "internal error: " + msg
		}
		http.Error(w, fmt.Sprintf(msg, args...), code)
	}

	if _, ok := verr.(*bakery.VerificationError); !ok {
		fail(http.StatusForbidden, "%v", verr)
		return
	}

	// Work out what caveats we need to apply for the given operation.
	// Could special-case the operation here if desired.
	caveats := []bakery.Caveat{
		checkers.TimeBefore(time.Now().Add(5 * time.Minute)),
		checkers.ThirdParty(srv.authEndpoint, "access-allowed"),
		checkers.FirstParty("operation " + operation),
	}
	// Mint an appropriate macaroon and send it back to the client.
	m, err := srv.svc.NewMacaroon("", nil, caveats)
	if err != nil {
		fail(http.StatusInternalServerError, "cannot mint macaroon: %v", err)
		return
	}
	httpbakery.WriteDischargeRequiredError(w, m, verr)
}
