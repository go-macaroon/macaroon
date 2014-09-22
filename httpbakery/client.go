package httpbakery

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"

	"github.com/rogpeppe/macaroon"
)

// Do makes an http request to the given client.
// If the request fails with a discharge-required error,
// any required discharge macaroons will be acquired,
// and the request will be repeated with those attached.
//
// If c.Jar field is non-nil, the macaroons will be
// stored there and made available to subsequent requests.
func Do(c *http.Client, req *http.Request) (*http.Response, error) {
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

	var resp dischargeRequestedResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
		return nil, fmt.Errorf("cannot unmarshal discharge-required response: %v", err)
	}
	if resp.ErrorCode != codeDischargeRequired {
		return nil, fmt.Errorf("unexpected error code: %q", resp.ErrorCode)
	}
	if resp.Macaroon == nil {
		return nil, fmt.Errorf("no macaroon found in response")
	}
	macaroons, err := dischargeMacaroon(c, resp.Macaroon)
	if err != nil {
		return nil, err
	}

	// Bind the discharge macaroons to the original macaroon.
	for _, m := range macaroons {
		m.Bind(resp.Macaroon.Signature())
	}
	macaroons = append(macaroons, resp.Macaroon)
	for _, m := range macaroons {
		if err := addCookie(req, m); err != nil {
			return nil, fmt.Errorf("cannot add cookie: %v", err)
		}
	}
	// Try again with our newly acquired discharge macaroons
	return c.Do(req)
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
			return nil, fmt.Errorf("cannot obtain discharge from %q: %v", cav.Location, err)
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
		return fmt.Errorf("POST %q failed with status %q (body %q)", url, httpResp.Status, data)
	}
	if err := json.Unmarshal(data, resp); err != nil {
		return fmt.Errorf("cannot unmarshal response from %q: %v", url, err)
	}
	return nil
}
