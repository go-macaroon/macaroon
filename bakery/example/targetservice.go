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

type myServer struct {
	svc          *httpbakery.Service
	authEndpoint string
	endpoint     string
}

func targetService(endpoint, authEndpoint string) (http.Handler, error) {
	svc, err := httpbakery.NewService(httpbakery.NewServiceParams{
		Location: endpoint,
	})
	if err != nil {
		return nil, err
	}
	return &myServer{
		svc:          svc,
		authEndpoint: authEndpoint,
	}, nil
}

func (srv *myServer) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	breq := srv.svc.NewRequest(req, srv.checkers(req))
	if err := breq.Check("can-access-me"); err != nil {
		srv.writeError(w, err)
		return
	}
	fmt.Fprintf(w, "hello, world\n")
}

func (svc *myServer) checkers(req *http.Request) bakery.FirstPartyChecker {
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
	}
	return checkers.PushFirstPartyChecker(m, checkers.Std)
}

func (srv *myServer) writeError(w http.ResponseWriter, err error) {
	fail := func(code int, msg string, args ...interface{}) {
		if code == http.StatusInternalServerError {
			msg = "internal error: " + msg
		}
		http.Error(w, fmt.Sprintf(msg, args...), code)
	}

	verr, _ := err.(*bakery.VerificationError)
	if verr == nil {
		fail(http.StatusForbidden, "%v", err)
		return
	}

	// Work out what caveats we need to apply for the given capability.
	var caveats []bakery.Caveat
	switch verr.RequiredCapability {
	case "can-access-me":
		caveats = []bakery.Caveat{
			checkers.TimeBefore(time.Now().Add(5 * time.Minute)),
			checkers.ThirdParty(srv.authEndpoint, "access-allowed"),
		}
	default:
		fail(http.StatusInternalServerError, "capability %q not recognised", verr.RequiredCapability)
		return
	}
	// Mint an appropriate macaroon and send it back to the client.
	m, err := srv.svc.NewMacaroon("", nil, verr.RequiredCapability, caveats)
	if err != nil {
		fail(http.StatusInternalServerError, "cannot mint macaroon: %v", err)
		return
	}
	httpbakery.WriteDischargeRequiredError(w, m, verr)
}
