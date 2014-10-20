package bakery

import (
	"fmt"

	"github.com/juju/errgo"

	"github.com/rogpeppe/macaroon"
)

// NewMacaroon mints a new macaroon with the given id and caveats.
// If the id is empty, a random id will be used.
// If rootKey is nil, a random root key will be used.
type NewMacarooner interface {
	NewMacaroon(id string, rootKey []byte, caveats []Caveat) (*macaroon.Macaroon, error)
}

// A Discharger can be used to discharge third party caveats.
type Discharger struct {
	// Checker is used to check the caveat's condition.
	Checker ThirdPartyChecker

	// Decoder is used to decode the caveat id.
	Decoder CaveatIdDecoder

	// Factory is used to create the macaroon.
	// Note that *Service implements NewMacarooner.
	Factory NewMacarooner
}

// Discharge creates a macaroon that discharges the third party
// caveat with the given id. The id should have been created
// earlier with a matching CaveatIdEncoder.
// The condition implicit in the id is checked for validity
// using d.Checker, and then if valid, a new macaroon
// is minted which discharges the caveat, and
// can eventually be associated with a client request using
// AddClientMacaroon.
func (d *Discharger) Discharge(id string) (*macaroon.Macaroon, error) {
	logf("server attempting to discharge %q", id)
	rootKey, condition, err := d.Decoder.DecodeCaveatId(id)
	if err != nil {
		return nil, fmt.Errorf("discharger cannot decode caveat id: %v", err)
	}
	caveats, err := d.Checker.CheckThirdPartyCaveat(id, condition)
	if err != nil {
		return nil, err
	}
	return d.Factory.NewMacaroon(id, rootKey, caveats)
}

// DischargeAll gathers discharge macaroons for all the third party caveats
// in m (and any subsequent caveats required by those) using getDischarge to
// acquire each discharge macaroon.
func DischargeAll(
	m *macaroon.Macaroon,
	getDischarge func(firstPartyLocation string, cav macaroon.Caveat) (*macaroon.Macaroon, error),
) ([]*macaroon.Macaroon, error) {
	var discharges []*macaroon.Macaroon
	var need []macaroon.Caveat
	addCaveats := func(m *macaroon.Macaroon) {
		for _, cav := range m.Caveats() {
			if cav.Location == "" {
				continue
			}
			need = append(need, cav)
		}
	}
	addCaveats(m)
	firstPartyLocation := m.Location()
	for len(need) > 0 {
		cav := need[0]
		need = need[1:]
		dm, err := getDischarge(firstPartyLocation, cav)
		if err != nil {
			return nil, errgo.NoteMask(err, fmt.Sprintf("cannot get discharge from %q", cav.Location), errgo.Any)
		}
		discharges = append(discharges, dm)
		addCaveats(dm)
	}
	return discharges, nil
}
