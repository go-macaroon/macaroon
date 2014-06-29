package bakery

import (
	"fmt"
	"log"

	"github.com/rogpeppe/macaroon"
)

// NewMacaroon mints a new macaroon with the given id, capability and caveats.
// If the id is empty, a random id will be used.
// If rootKey is nil, a random root key will be used.
type NewMacarooner interface {
	NewMacaroon(id string, rootKey []byte, capability string, caveats []Caveat) (*macaroon.Macaroon, error)
}

// A Discharger can be used to discharge third party macaroons
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
// caveat with the given id.
func (d *Discharger) Discharge(id string) (*macaroon.Macaroon, error) {
	log.Printf("server attempting to discharge %q", id)
	rootKey, condition, err := d.Decoder.DecodeCaveatId(id)
	if err != nil {
		return nil, fmt.Errorf("discharger cannot decode caveat id: %v", err)
	}
	caveats, err := d.Checker.CheckThirdPartyCaveat(condition)
	if err != nil {
		return nil, err
	}
	return d.Factory.NewMacaroon(id, rootKey, "", caveats)
}
