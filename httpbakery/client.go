package httpbakery

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"

	"code.google.com/p/go.net/publicsuffix"
	"github.com/juju/errgo"

	"github.com/rogpeppe/macaroon"
	"github.com/rogpeppe/macaroon/bakery"
)

// WaitResponse holds the type that should be returned
// by an HTTP response made to a WaitURL
// (See the ErrorInfo type).
type WaitResponse struct {
	Macaroon *macaroon.Macaroon
}

// Do makes an http request to the given client.
// If the request fails with a discharge-required error,
// any required discharge macaroons will be acquired,
// and the request will be repeated with those attached.
//
// If the client.Jar field is non-nil, the macaroons will be
// stored there and made available to subsequent requests.
func Do(client *http.Client, req *http.Request, visitWebPage func(url *url.URL) error) (*http.Response, error) {
	// Add a temporary cookie jar (without mutating the original
	// client) if there isn't one available.
	if client.Jar == nil {
		client1 := *client
		jar, err := cookiejar.New(&cookiejar.Options{
			PublicSuffixList: publicsuffix.List,
		})
		if err != nil {
			return nil, errgo.Notef(err, "cannot make cookie jar")
		}
		client1.Jar = jar
		client = &client1
	}
	ctxt := &clientContext{
		client:       client,
		visitWebPage: visitWebPage,
	}
	return ctxt.do(req)
}

type clientContext struct {
	client       *http.Client
	visitWebPage func(*url.URL) error
}

// relativeURL returns newPath relative to an original URL.
func relativeURL(base, new string) (*url.URL, error) {
	if new == "" {
		return nil, errgo.Newf("empty URL")
	}
	baseURL, err := url.Parse(base)
	if err != nil {
		return nil, errgo.Notef(err, "cannot parse URL")
	}
	newURL, err := url.Parse(new)
	if err != nil {
		return nil, errgo.Notef(err, "cannot parse URL")
	}
	return baseURL.ResolveReference(newURL), nil
}

func (ctxt *clientContext) do(req *http.Request) (*http.Response, error) {
	log.Printf("client do %s %s {", req.Method, req.URL)
	resp, err := ctxt.do1(req)
	log.Printf("} -> error %#v", err)
	return resp, err
}

func (ctxt *clientContext) do1(req *http.Request) (*http.Response, error) {
	httpResp, err := ctxt.client.Do(req)
	if err != nil {
		return nil, err
	}
	if httpResp.StatusCode != http.StatusProxyAuthRequired {
		return httpResp, nil
	}
	if httpResp.Header.Get("Content-Type") != "application/json" {
		return httpResp, nil
	}
	defer httpResp.Body.Close()

	var resp Error
	if err := json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
		return nil, errgo.Notef(err, "cannot unmarshal error response")
	}
	if resp.Code != ErrDischargeRequired {
		return nil, errgo.NoteMask(&resp, fmt.Sprintf("%s %s failed", req.Method, req.URL), errgo.Any)
	}
	if resp.Info == nil || resp.Info.Macaroon == nil {
		return nil, errgo.New("no macaroon found in response")
	}
	mac := resp.Info.Macaroon
	macaroons, err := bakery.DischargeAll(mac, ctxt.obtainThirdPartyDischarge)
	if err != nil {
		return nil, err
	}
	// Bind the discharge macaroons to the original macaroon.
	for _, m := range macaroons {
		m.Bind(mac.Signature())
	}
	// TODO(rog) perhaps we should add all the macaroons as a single
	// cookie, with the principal macaroon first.
	macaroons = append(macaroons, mac)
	if err := ctxt.addCookies(req, macaroons); err != nil {
		return nil, errgo.Notef(err, "cannot add cookie")
	}
	// Try again with our newly acquired discharge macaroons
	hresp, err := ctxt.client.Do(req)
	return hresp, err
}

func (ctxt *clientContext) addCookies(req *http.Request, ms []*macaroon.Macaroon) error {
	var cookies []*http.Cookie
	for _, m := range ms {
		data, err := m.MarshalJSON()
		if err != nil {
			return errgo.Notef(err, "cannot marshal macaroon")
		}
		cookies = append(cookies, &http.Cookie{
			Name:  fmt.Sprintf("macaroon-%x", m.Signature()),
			Value: base64.StdEncoding.EncodeToString(data),
			// TODO(rog) other fields
		})
	}
	// TODO should we set it for the URL only, or the host.
	// Can we set cookies such that they'll always get sent to any
	// URL on the given host?
	ctxt.client.Jar.SetCookies(req.URL, cookies)
	return nil
}

func (ctxt *clientContext) obtainThirdPartyDischarge(originalLocation string, cav macaroon.Caveat) (*macaroon.Macaroon, error) {
	var resp dischargeResponse
	loc := appendURLElem(cav.Location, "discharge")
	err := postFormJSON(
		loc,
		url.Values{
			"id":       {cav.Id},
			"location": {originalLocation},
		},
		&resp,
		ctxt.postForm,
	)
	if err == nil {
		return resp.Macaroon, nil
	}
	log.Printf("discharge post got error %#v", err)
	cause, ok := errgo.Cause(err).(*Error)
	if !ok {
		return nil, errgo.Notef(err, "cannot acquire discharge")
	}
	if cause.Code != ErrInteractionRequired {
		return nil, errgo.Mask(err)
	}
	if cause.Info == nil {
		return nil, errgo.Notef(err, "interaction-required response with no info")
	}
	return ctxt.interact(loc, cause.Info.VisitURL, cause.Info.WaitURL)
}

// interact gathers a macaroon by directing the user to interact
// with a web page.
func (ctxt *clientContext) interact(location, visitURLStr, waitURLStr string) (*macaroon.Macaroon, error) {
	visitURL, err := relativeURL(location, visitURLStr)
	if err != nil {
		return nil, errgo.Notef(err, "cannot make relative visit URL")
	}
	waitURL, err := relativeURL(location, waitURLStr)
	if err != nil {
		return nil, errgo.Notef(err, "cannot make relative wait URL")
	}
	if err := ctxt.visitWebPage(visitURL); err != nil {
		return nil, errgo.Notef(err, "cannot start interactive session")
	}
	waitResp, err := ctxt.client.Get(waitURL.String())
	if err != nil {
		return nil, errgo.Notef(err, "cannot get %q", waitURL)
	}
	defer waitResp.Body.Close()
	if waitResp.StatusCode != http.StatusOK {
		var resp Error
		if err := json.NewDecoder(waitResp.Body).Decode(&resp); err != nil {
			return nil, errgo.Notef(err, "cannot unmarshal wait error response")
		}
		return nil, errgo.NoteMask(&resp, "failed to acquire macaroon after waiting", errgo.Any)
	}
	var resp WaitResponse
	if err := json.NewDecoder(waitResp.Body).Decode(&resp); err != nil {
		return nil, errgo.Notef(err, "cannot unmarshal wait response")
	}
	if resp.Macaroon == nil {
		return nil, errgo.New("no macaroon found in wait response")
	}
	return resp.Macaroon, nil
}

func (ctxt *clientContext) postForm(url string, data url.Values) (*http.Response, error) {
	log.Printf("clientContext.postForm {")
	defer log.Printf("}")
	return ctxt.post(url, "application/x-www-form-urlencoded", strings.NewReader(data.Encode()))
}

func (ctxt *clientContext) post(url string, bodyType string, body io.Reader) (resp *http.Response, err error) {
	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", bodyType)
	// TODO(rog) see http.shouldRedirectPost
	return ctxt.do(req)
}

// postFormJSON does an HTTP POST request to the given url with the given
// values and unmarshals the response in the value pointed to be resp.
// It uses the given postForm function to actually make the POST request.
func postFormJSON(url string, vals url.Values, resp interface{}, postForm func(url string, vals url.Values) (*http.Response, error)) error {
	log.Printf("postFormJSON to %s; vals: %#v", url, vals)
	httpResp, err := postForm(url, vals)
	if err != nil {
		return errgo.NoteMask(err, fmt.Sprintf("cannot http POST to %q", url), errgo.Any)
	}
	defer httpResp.Body.Close()
	data, err := ioutil.ReadAll(httpResp.Body)
	if err != nil {
		return errgo.Notef(err, "failed to read body from %q", url)
	}
	if httpResp.StatusCode != http.StatusOK {
		var errResp Error
		if err := json.Unmarshal(data, &errResp); err != nil {
			// TODO better error here
			return errgo.Notef(err, "POST %q failed with status %q; cannot parse body %q", url, httpResp.Status, data)
		}
		return &errResp
	}
	if err := json.Unmarshal(data, resp); err != nil {
		return errgo.Notef(err, "cannot unmarshal response from %q", url)
	}
	return nil
}
