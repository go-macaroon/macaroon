package httpbakery

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"path"

	"github.com/juju/errgo"

	"github.com/rogpeppe/macaroon"
	"github.com/rogpeppe/macaroon/bakery"
)

type dischargeHandler struct {
	svc     *Service
	checker func(req *http.Request, cavId, cav string) ([]bakery.Caveat, error)
}

// AddDischargeHandler handles adds handlers to the given ServeMux
// under the given root path to service third party caveats.
// If rootPath is empty, "/" will be used.
//
// The check function is used to check whether a client making the given
// request should be allowed a discharge for the given caveat. If it
// does not return an error, the caveat will be discharged, with any
// returned caveats also added to the discharge macaroon.
// If it returns an error with a *Error cause, the error will be marshaled
// and sent back to the client.
//
// The name space served by DischargeHandler is as follows.
// All parameters can be provided either as URL attributes
// or form attributes. The result is always formatted as a JSON
// object.
//
// On failure, all endpoints return an error described by
// the Error type.
//
// POST /discharge
//	params:
//		id: id of macaroon to discharge
//		location: location of original macaroon (optional (?))
//		?? flow=redirect|newwindow
//	result on success (http.StatusOK):
//		{
//			Macaroon *macaroon.Macaroon
//		}
//
// POST /create
//	params:
//		condition: caveat condition to discharge
//		rootkey: root key of discharge caveat
//	result:
//		{
//			CaveatID: string
//		}
//
// GET /publickey
//	result:
//		public key of service
//		expiry time of key
func (svc *Service) AddDischargeHandler(
	rootPath string,
	mux *http.ServeMux,
	checker func(req *http.Request, cavId, cav string) ([]bakery.Caveat, error),
) {
	d := &dischargeHandler{
		svc:     svc,
		checker: checker,
	}
	if rootPath == "" {
		rootPath = "/"
	}
	mux.Handle(path.Join(rootPath, "discharge"), handleJSON(d.serveDischarge))
	mux.Handle(path.Join(rootPath, "create"), handleJSON(d.serveCreate))
	// TODO(rog) is there a case for making public key caveat signing
	// optional?
	mux.Handle(path.Join(rootPath, "publickey"), handleJSON(d.servePublicKey))
}

type dischargeResponse struct {
	Macaroon *macaroon.Macaroon `json:",omitempty"`
}

func (d *dischargeHandler) serveDischarge(w http.ResponseWriter, req *http.Request) (interface{}, error) {
	if req.Method != "POST" {
		// TODO http.StatusMethodNotAllowed)
		return nil, badRequestErrorf("method not allowed")
	}
	req.ParseForm()
	id := req.Form.Get("id")
	if id == "" {
		return nil, badRequestErrorf("id attribute is empty")
	}
	checker := func(cavId, cav string) ([]bakery.Caveat, error) {
		return d.checker(req, cavId, cav)
	}
	discharger := d.svc.Discharger(bakery.ThirdPartyCheckerFunc(checker))

	// TODO(rog) pass location into discharge
	// location := req.Form.Get("location")

	var resp dischargeResponse
	m, err := discharger.Discharge(id)
	if err != nil {
		return nil, errgo.NoteMask(err, "cannot discharge", errgo.Any)
	} else {
		resp.Macaroon = m
	}
	return &resp, nil
}

func (d *dischargeHandler) internalError(w http.ResponseWriter, f string, a ...interface{}) {
	http.Error(w, fmt.Sprintf(f, a...), http.StatusInternalServerError)
}

func (d *dischargeHandler) badRequest(w http.ResponseWriter, f string, a ...interface{}) {
	http.Error(w, fmt.Sprintf(f, a...), http.StatusBadRequest)
}

type thirdPartyCaveatIdRecord struct {
	RootKey   []byte
	Condition string
}

func (d *dischargeHandler) serveCreate(w http.ResponseWriter, req *http.Request) (interface{}, error) {
	req.ParseForm()
	condition := req.Form.Get("condition")
	rootKeyStr := req.Form.Get("root-key")

	if len(condition) == 0 {
		return nil, badRequestErrorf("empty value for condition")
	}
	if len(rootKeyStr) == 0 {
		return nil, badRequestErrorf("empty value for root key")
	}
	rootKey, err := base64.StdEncoding.DecodeString(rootKeyStr)
	if err != nil {
		return nil, badRequestErrorf("cannot base64-decode root key: %v", err)
	}
	// TODO(rog) what about expiry times?
	idBytes, err := randomBytes(24)
	if err != nil {
		return nil, fmt.Errorf("cannot generate random key: %v", err)
	}
	id := fmt.Sprintf("%x", idBytes)
	recordBytes, err := json.Marshal(thirdPartyCaveatIdRecord{
		Condition: condition,
		RootKey:   rootKey,
	})
	if err != nil {
		return nil, fmt.Errorf("cannot marshal caveat id record: %v", err)
	}
	err = d.svc.Store().Put(id, string(recordBytes))
	if err != nil {
		return nil, fmt.Errorf("cannot store caveat id record: %v", err)
	}
	return caveatIdResponse{
		CaveatId: id,
	}, nil
}

func (d *dischargeHandler) servePublicKey(w http.ResponseWriter, r *http.Request) (interface{}, error) {
	return nil, fmt.Errorf("not implemented yet")
}

func randomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	if err != nil {
		return nil, fmt.Errorf("cannot generate %d random bytes: %v", n, err)
	}
	return b, nil
}
