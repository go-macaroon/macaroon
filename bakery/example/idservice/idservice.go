package idservice

import (
	"encoding/base64"
	"fmt"
	"html/template"
	"log"
	"net/http"

	"github.com/juju/errgo"
	"github.com/juju/utils/jsonhttp"

	"github.com/rogpeppe/macaroon"
	"github.com/rogpeppe/macaroon/bakery"
	"github.com/rogpeppe/macaroon/bakery/checkers"
	"github.com/rogpeppe/macaroon/bakery/example/meeting"
	"github.com/rogpeppe/macaroon/httpbakery"
)

var (
	handleJSON = jsonhttp.HandleJSON(errorToResponse)
)

const (
	cookieUser = "username"
)

// handler implements http.Handler to serve the name space
// provided by the id service.
type handler struct {
	svc   *httpbakery.Service
	place *place
	users map[string]*UserInfo
}

// UserInfo holds information about a user.
type UserInfo struct {
	Password string
	Groups   map[string]bool
}

// Params holds parameters for New.
type Params struct {
	Service httpbakery.NewServiceParams
	Users   map[string]*UserInfo
}

// New returns a new handler that services an identity-providing
// service. This acts as a login service and can discharge third-party caveats
// for users.
func New(p Params) (http.Handler, error) {
	svc, err := httpbakery.NewService(p.Service)
	if err != nil {
		return nil, err
	}
	h := &handler{
		svc:   svc,
		users: p.Users,
		place: &place{meeting.New()},
	}
	mux := http.NewServeMux()
	svc.AddDischargeHandler("/", mux, h.checkThirdPartyCaveat)
	mux.Handle("/user/", handleJSON(h.userHandler))
	mux.HandleFunc("/login", h.loginHandler)
	mux.Handle("/question", handleJSON(h.questionHandler))
	mux.Handle("/wait", handleJSON(h.waitHandler))
	mux.HandleFunc("/loginattempt", h.loginAttemptHandler)
	return mux, nil
}

// userHandler handles requests to add new users, change user details, etc.
// It is only accessible to users that are members of the admin group.
func (h *handler) userHandler(w http.ResponseWriter, req *http.Request) (interface{}, error) {
	ctxt := h.newContext(req, "change-user")
	breq := h.svc.NewRequest(req, ctxt)
	err := breq.Check()
	if err != nil {
		// We issue a macaroon with a third-party caveat targetting
		// the id service itself. This means that the flow for self-created
		// macaroons is just the same as for any other service.
		// Theoretically, we could just redirect the user to the
		// login page, but that would require a different flow
		// and it's not clear that it would be an advantage.
		m, err := h.svc.NewMacaroon("", nil, []bakery.Caveat{
			checkers.ThirdParty(h.svc.Location(), "member-of-group admin"),
			checkers.FirstParty("operation change-user"),
		})
		if err != nil {
			return nil, errgo.Notef(err, "cannot mint new macaroon")
		}
		return nil, &httpbakery.Error{
			Message: err.Error(),
			Code:    httpbakery.ErrDischargeRequired,
			Info: &httpbakery.ErrorInfo{
				Macaroon: m,
			},
		}
	}
	// PUT /user/$user - create new user
	// PUT /user/$user/group-membership - change group membership of user
	return nil, errgo.New("not implemented yet")
}

type loginPageParams struct {
	WaitId string
}

var loginPage = template.Must(template.New("").Parse(`
<html>
<body>
<form action="/loginattempt" method="POST">
User name: <input type="text" name="user"></input>
<p>
Password: <input type="password" name="password"></input>
<input type="submit">Log in</input>
<input type="hidden" name="waitid" value="{{.WaitId}}"></input>
</form>
</body>
</html>
`))

// loginHandler serves up a login page for the user to interact with,
// having been redirected there as part of a macaroon discharge requirement.
// This is a proxy for any third-party authorization service.
func (h *handler) loginHandler(w http.ResponseWriter, req *http.Request) {
	req.ParseForm()
	waitId := req.Form.Get("waitid")
	if waitId == "" {
		http.Error(w, "wait id not found in form", http.StatusBadRequest)
		return
	}
	err := loginPage.Execute(w, loginPageParams{
		WaitId: waitId,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// loginAttemptHandler is invoked when a user clicks on the "Log in"
// button on the login page. It checks the credentials and then
// completes the rendezvous, allowing the original wait
// request to complete.
func (h *handler) loginAttemptHandler(w http.ResponseWriter, req *http.Request) {
	log.Printf("login attempt %s", req.URL)
	req.ParseForm()
	waitId := req.Form.Get("waitid")
	if waitId == "" {
		http.Error(w, "wait id not found in form", http.StatusBadRequest)
		return
	}
	user := req.Form.Get("user")
	info, ok := h.users[user]
	if !ok {
		http.Error(w, fmt.Sprintf("user %q not found", user), http.StatusUnauthorized)
		return
	}
	if req.Form.Get("password") != info.Password {
		http.Error(w, "bad password", http.StatusUnauthorized)
		return
	}

	// User and password match; we can allow the user
	// to have a macaroon that they can use later to prove
	// to us that they have logged in. We also add a cookie
	// to hold the logged in user name.
	m, err := h.svc.NewMacaroon("", nil, []bakery.Caveat{{
		Condition: "user-is " + user,
	}})
	// TODO(rog) when this fails, we should complete the rendezvous
	// to cause the wait request to complete with an appropriate error.
	if err != nil {
		http.Error(w, "cannot mint macaroon: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := addMacaroonAsCookie(w, m); err != nil {
		http.Error(w, "cannot add macaroon cookie", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Path:  "/",
		Name:  cookieUser,
		Value: user,
	})
	h.place.Done(waitId, &loginInfo{
		User: user,
	})
}

func addMacaroonAsCookie(w http.ResponseWriter, m *macaroon.Macaroon) error {
	data, err := m.MarshalJSON()
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Path:  "/",
		Name:  fmt.Sprintf("macaroon-%x", m.Signature()),
		Value: base64.StdEncoding.EncodeToString(data),
		// TODO(rog) other fields
	})
	return nil
}

// checkThirdPartyCaveat is called by the httpbakery discharge handler.
func (h *handler) checkThirdPartyCaveat(req *http.Request, cavId, cav string) ([]bakery.Caveat, error) {
	return h.newContext(req, "").CheckThirdPartyCaveat(cavId, cav)
}

// newContext returns a new caveat-checking context
// for the client making the given request.
func (h *handler) newContext(req *http.Request, operation string) *context {
	// Determine the current logged-in user, if any.
	var username string
	for _, c := range req.Cookies() {
		if c.Name == cookieUser {
			// TODO could potentially allow several concurrent
			// logins - caveats asking about current user privilege
			// could be satisfied if any of the user names had that
			// privilege.
			username = c.Value
			break
		}
	}
	if username == "" {
		log.Printf("not logged in")
	} else {
		log.Printf("logged in as %q", username)
	}
	return &context{
		handler:      h,
		req:          req,
		declaredUser: username,
		operation:    operation,
	}
}

// needLogin returns an error suitable for returning
// from a discharge request that can only be satisfied
// if the user logs in.
func (h *handler) needLogin(cavId string, caveat string, why string) error {
	// TODO(rog) If the user is already logged in (username != ""),
	// we should perhaps just return an error here.
	log.Printf("login required")
	waitId, err := h.place.NewRendezvous(&thirdPartyCaveatInfo{
		CaveatId: cavId,
		Caveat:   caveat,
	})
	if err != nil {
		return fmt.Errorf("cannot make rendezvous: %v", err)
	}
	log.Printf("returning redirect error")
	return &httpbakery.Error{
		Message: why,
		Code:    httpbakery.ErrInteractionRequired,
		Info: &httpbakery.ErrorInfo{
			VisitURL: "/login?waitid=" + waitId,
			WaitURL:  "/wait?waitid=" + waitId,
		},
	}
}

// waitHandler serves an HTTP endpoint that waits until a macaroon
// has been discharged, and returns the discharge macaroon.
func (h *handler) waitHandler(w http.ResponseWriter, req *http.Request) (interface{}, error) {
	req.ParseForm()
	waitId := req.Form.Get("waitid")
	if waitId == "" {
		return nil, fmt.Errorf("wait id parameter not found")
	}
	caveat, login, err := h.place.Wait(waitId)
	if err != nil {
		return nil, fmt.Errorf("cannot wait: %v", err)
	}
	if login.User == "" {
		return nil, fmt.Errorf("login failed")
	}
	// Create a context to verify the third party caveat.
	// Note that because the information in login has been
	// supplied directly by our own code, we can assume
	// that it can be trusted, so we set verifiedUser to true.
	ctxt := &context{
		handler:      h,
		req:          req,
		declaredUser: login.User,
		verifiedUser: true,
	}
	// Now that we've verified the user, we can check again to see
	// if we can discharge the original caveat.
	discharger := h.svc.Discharger(ctxt)
	macaroon, err := discharger.Discharge(caveat.CaveatId)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	return WaitResponse{
		Macaroon: macaroon,
	}, nil
}

func (h *handler) questionHandler(w http.ResponseWriter, req *http.Request) (interface{}, error) {
	return nil, errgo.New("question unimplemented")
	// TODO
	//	req.ParseForm()
	//
	//	macStr := req.Form.Get("macaroons")
	//	if macStr == "" {
	//		return nil, fmt.Errorf("macaroon parameter not found")
	//	}
	//	var macaroons []*macaroon.Macaroon
	//	err := json.Unmarshal([]byte(macStr), &macaroons)
	//	if err != nil {
	//		return nil, fmt.Errorf("cannot unmarshal macaroon: %v", err)
	//	}
	//	if len(macaroons) == 0 {
	//		return nil, fmt.Errorf("no macaroons found")
	//	}
	//	q := req.Form.Get("q")
	//	if q == "" {
	//		return nil, fmt.Errorf("q parameter not found")
	//	}
	//	user := req.Form.Get("user")
	//	if user == "" {
	//		return nil, fmt.Errorf("user parameter not found")
	//	}
	//	ctxt := &context{
	//		declaredUser: user,
	//		operation: "question " + q,
	//	}
	//	breq := h.svc.NewRequest(req, ctxt)
	//	for _, m := range macaroons {
	//		breq.AddClientMacaroon(m)
	//	}
	//	err := breq.Check()
	//	return nil, err
}

// WaitResponse holds the response from the wait endpoint.
type WaitResponse struct {
	Macaroon *macaroon.Macaroon
}

// context represents the context in which a caveat
// will be checked.
type context struct {
	// handler refers to the idservice handler.
	handler *handler

	// declaredUser holds the user name that we want to use for
	// checking authorization caveats.
	declaredUser string

	// verifiedUser is true when the declared user has been verified
	// directly (by the user login)
	verifiedUser bool

	// operation holds the current operation, if any.
	operation string

	// req holds the current client's HTTP request.
	req *http.Request
}

func (ctxt *context) CheckFirstPartyCaveat(caveat string) error {
	op, rest, err := checkers.ParseCaveat(caveat)
	if err != nil {
		return fmt.Errorf("cannot parse caveat %q: %v", caveat, err)
	}
	switch op {
	case "user-is":
		if rest != ctxt.declaredUser {
			return fmt.Errorf("not logged in as %q", rest)
		}
		return nil
	case "operation":
		if ctxt.operation != "" && rest == ctxt.operation {
			return nil
		}
		return errgo.Newf("operation mismatch")
	default:
		return &bakery.CaveatNotRecognizedError{caveat}
	}
}

func (ctxt *context) CheckThirdPartyCaveat(cavId, cav string) ([]bakery.Caveat, error) {
	h := ctxt.handler
	log.Printf("checking third party caveat %q", cav)
	op, rest, err := checkers.ParseCaveat(cav)
	if err != nil {
		return nil, fmt.Errorf("cannot parse caveat %q: %v", cav, err)
	}
	switch op {
	case "can-speak-for":
		// TODO(rog) We ignore the currently logged in user here,
		// but perhaps it would be better to let the user be in control
		// of which user they're currently "declared" as, rather than
		// getting privileges of users we currently have macaroons for.
		checkErr := ctxt.canSpeakFor(rest)
		if checkErr == nil {
			return ctxt.firstPartyCaveats(), nil
		}
		return nil, h.needLogin(cavId, cav, checkErr.Error())
	case "member-of-group":
		// The third-party caveat is asking if the currently logged in
		// user is a member of a particular group.
		// We can find the currently logged in user by checking
		// the username cookie (which doesn't provide any power, but
		// indicates which user name to check)
		if ctxt.declaredUser == "" {
			return nil, h.needLogin(cavId, cav, "not logged in")
		}
		if err := ctxt.canSpeakFor(ctxt.declaredUser); err != nil {
			return nil, errgo.Notef(err, "cannot speak for declared user %q", ctxt.declaredUser)
		}
		info, ok := h.users[ctxt.declaredUser]
		if !ok {
			return nil, errgo.Newf("user %q not found", ctxt.declaredUser)
		}
		group := rest
		if !info.Groups[group] {
			return nil, errgo.Newf("not privileged enough")
		}
		return ctxt.firstPartyCaveats(), nil
	default:
		return nil, &bakery.CaveatNotRecognizedError{cav}
	}
}

// canSpeakFor checks whether the client sending
// the given request can speak for the given user.
// We do that by declaring that user and checking
// whether the supplied macaroons in the request
// verify OK.
func (ctxt *context) canSpeakFor(user string) error {
	if user == ctxt.declaredUser && ctxt.verifiedUser {
		// The context is a direct result of logging in.
		// No need to check macaroons.
		return nil
	}
	ctxt1 := *ctxt
	ctxt1.declaredUser = user
	breq := ctxt.handler.svc.NewRequest(ctxt.req, &ctxt1)
	err := breq.Check()
	if err != nil {
		log.Printf("client cannot speak for %q: %v", user, err)
	} else {
		log.Printf("client can speak for %q", user)
	}
	return err
}

// firstPartyCaveats returns first-party caveats suitable
// for adding to a third-party caveat discharge macaroon
// within the receiving context.
func (ctxt *context) firstPartyCaveats() []bakery.Caveat {
	// TODO return caveat specifying that ip-addr is
	// the same as that given in ctxt.req.RemoteAddr
	// and other 1st party caveats, potentially.
	return nil
}

func errorToResponse(err error) (int, interface{}) {
	cause := errgo.Cause(err)
	if cause, ok := cause.(*httpbakery.Error); ok {
		err1 := *cause
		err1.Message = err.Error()
		return http.StatusInternalServerError, &err1
	}
	return http.StatusInternalServerError, &httpbakery.Error{
		Message: err.Error(),
	}
}
