package httpbakery

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/rogpeppe/macaroon"
)

type dischargeRequestedResponse struct {
	Error     string
	ErrorCode string
	Macaroon  *macaroon.Macaroon
}

const codeDischargeRequired = "macaroon discharge required"

// WriteDischargeRequiredError writes a response to w that reports the
// given error and sends the given macaroon to the client, indicating
// that it should be discharged to allow the original request to be
// accepted.
//
// If it returns an error, it will have written the http response
// anyway.
//
// The cookie value is a base-64-encoded JSON serialization of the
// macaroon.
//
// TODO(rog) consider an alternative approach - perhaps
// it would be better to include the macaroon directly in the
// response and leave it up to the client to add it to the cookies
// along with the discharge macaroons.
func WriteDischargeRequiredError(w http.ResponseWriter, m *macaroon.Macaroon, originalErr error) error {
	if originalErr == nil {
		originalErr = fmt.Errorf("unauthorized")
	}
	respData, err := json.Marshal(dischargeRequestedResponse{
		Error:     originalErr.Error(),
		ErrorCode: codeDischargeRequired,
		Macaroon:  m,
	})
	if err != nil {
		err = fmt.Errorf("internal error: cannot marshal response: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusProxyAuthRequired)
	w.Write(respData)
	return nil
}

// It remains to be seen whether the following code is useful
// in practice:

//var (
//	requestMutex sync.Mutex
//	requests     map[*http.Request]*Request
//)
//
//// NewHandler returns an http handler that wraps the given
//// handler by creating a Request for each http.Request
//// that can be retrieved by calling GetRequest.
//func NewHandler(svc *bakery.Service, handler http.Handler) http.Handler {
//}
//
//// BakeryRequest wraps *bakery.Request. It is
//// defined to avoid a field clash in the definition
//// of Request.
//type BakeryRequest struct {
//	*bakery.Request
//}
//
//// Request holds a request invoked through a handler returned
//// by NewHandler. It wraps the original http request and the
//// associated bakery request.
//type Request struct {
//	*http.Request
//	BakeryRequest
//}
//
//// GetRequest retrieves the request for the given http request,
//// which must have be a currently outstanding request
//// invoked through a handler returned by NewHandler.
//// It panics if there is no associated request.
//func GetRequest(req *http.Request) *Request
//
//type FirstPartyCaveat func(req *http.Request, caveat string) error
//type ThirdPartyCaveat func(req *http.Request, caveat string) ([]bakery.Caveat, error)
