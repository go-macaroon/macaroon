package httpbakery

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"

	"github.com/juju/errgo"

	"github.com/rogpeppe/macaroon"
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
// If c.Jar field is non-nil, the macaroons will be
// stored there and made available to subsequent requests.
func Do(c *http.Client, req *http.Request, visitWebPage func(url string) error) (*http.Response, error) {
	httpResp, err := c.Do(req)
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
		return nil, errgo.Notef(err, "cannot unmarshal discharge-required response")
	}
	var mac *macaroon.Macaroon
	switch resp.Code {
	case ErrInteractionRequired:
		if resp.Info == nil {
			return nil, errgo.Notef(&resp, "interaction-required response with no info")
		}
		if err := visitWebPage(resp.Info.VisitURL); err != nil {
			return nil, errgo.Notef(err, "cannot start interactive session")
		}
		waitResp, err := c.Get(resp.Info.WaitURL)
		if err != nil {
			return nil, errgo.Notef(err, "cannot get %q: %v", resp.Info.WaitURL)
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
			return nil, fmt.Errorf("no macaroon found in wait response")
		}
		mac = resp.Macaroon
	case ErrDischargeRequired:
		if resp.Info == nil || resp.Info.Macaroon == nil {
			return nil, fmt.Errorf("no macaroon found in response")
		}
		mac = resp.Info.Macaroon
	default:
		return nil, errgo.NoteMask(&resp, fmt.Sprintf("%s %s failed", req.Method, req.URL), errgo.Any)
	}

	macaroons, err := dischargeMacaroon(c, mac)
	if err != nil {
		return nil, err
	}
	// Bind the discharge macaroons to the original macaroon.
	for _, m := range macaroons {
		m.Bind(mac.Signature())
	}
	macaroons = append(macaroons, mac)
	for _, m := range macaroons {
		if err := addCookie(req, m); err != nil {
			return nil, fmt.Errorf("cannot add cookie: %v", err)
		}
	}
	// Try again with our newly acquired discharge macaroons
	hresp, err := c.Do(req)
	return hresp, err
}

func addCookie(req *http.Request, m *macaroon.Macaroon) error {
	data, err := m.MarshalJSON()
	if err != nil {
		return err
	}
	req.AddCookie(&http.Cookie{
		Name:  fmt.Sprintf("macaroon-%x", m.Signature()),
		Value: base64.StdEncoding.EncodeToString(data),
		// TODO(rog) other fields
	})
	return nil
}

// dischargeMacaroon attempts to discharge all third-party caveats
// found in the given macaroon, returning the set of discharge
// macaroons.
func dischargeMacaroon(c *http.Client, m *macaroon.Macaroon) ([]*macaroon.Macaroon, error) {
	var macaroons []*macaroon.Macaroon
	for _, cav := range m.Caveats() {
		if cav.Location == "" {
			continue
		}
		m, err := obtainThirdPartyDischarge(c, m.Location(), cav)
		if err != nil {
			// TODO errgo.NoteMask... "cannot obtain discharge from %q
			return nil, err
		}
		macaroons = append(macaroons, m)
	}
	return macaroons, nil
}

func obtainThirdPartyDischarge(c *http.Client, originalLocation string, cav macaroon.Caveat) (*macaroon.Macaroon, error) {
	var resp dischargeResponse
	if err := postFormJSON(
		c,
		appendURLElem(cav.Location, "discharge"),
		url.Values{
			"id":       {cav.Id},
			"location": {originalLocation},
		},
		&resp,
	); err != nil {
		return nil, err
	}
	return resp.Macaroon, nil
}

func postFormJSON(c *http.Client, url string, vals url.Values, resp interface{}) error {
	httpResp, err := c.PostForm(url, vals)
	if err != nil {
		return fmt.Errorf("cannot http POST to %q: %v", url, err)
	}
	defer httpResp.Body.Close()
	data, err := ioutil.ReadAll(httpResp.Body)
	if err != nil {
		return fmt.Errorf("failed to read body from %q: %v", url, err)
	}
	if httpResp.StatusCode != http.StatusOK {
		var errResp Error
		if err := json.Unmarshal(data, &errResp); err != nil {
			// TODO better error here
			return fmt.Errorf("POST %q failed with status %q; cannot parse body %q: %v", url, httpResp.Status, data, err)
		}
		return &errResp
	}
	if err := json.Unmarshal(data, resp); err != nil {
		return fmt.Errorf("cannot unmarshal response from %q: %v", url, err)
	}
	return nil
}
