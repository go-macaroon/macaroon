// The checkers package provides some standard caveat
// checkers and checker-combining functions.
package checkers

import (
	"fmt"
	"strings"
	"time"

	"github.com/rogpeppe/macaroon/bakery"
)

func FirstParty(condition string) bakery.Caveat {
	return bakery.Caveat{
		Condition: condition,
	}
}

func ThirdParty(location, condition string) bakery.Caveat {
	return bakery.Caveat{
		Location:  location,
		Condition: condition,
	}
}

var Std = Map{
	"time-before": bakery.FirstPartyCheckerFunc(timeBefore),
}

func TimeBefore(t time.Time) bakery.Caveat {
	return bakery.Caveat{
		Condition: "time-before " + t.Format(time.RFC3339),
	}
}

func timeBefore(cav string) error {
	_, timeStr, err := ParseCaveat(cav)
	if err != nil {
		return err
	}
	t, err := time.Parse(time.RFC3339, timeStr)
	if err != nil {
		return err
	}
	if time.Now().After(t) {
		return fmt.Errorf("after expiry time")
	}
	return nil
}

type Map map[string]bakery.FirstPartyCheckerFunc

func (m Map) CheckFirstPartyCaveat(cav string) error {
	id, _, err := ParseCaveat(cav)
	if err != nil {
		return fmt.Errorf("cannot parse caveat %q: %v", cav, err)
	}
	if c := m[id]; c != nil {
		return c.CheckFirstPartyCaveat(cav)
	}
	return &bakery.CaveatNotRecognizedError{cav}
}

// PushFirstPartyChecker returns a checker that first
// uses c0 to check caveats, and falls back to using c1
// if c0 returns bakery.ErrCaveatNotRecognized.
func PushFirstPartyChecker(c0, c1 bakery.FirstPartyChecker) bakery.FirstPartyChecker {
	f := func(caveat string) error {
		err := c0.CheckFirstPartyCaveat(caveat)
		if _, ok := err.(*bakery.CaveatNotRecognizedError); ok {
			err = c1.CheckFirstPartyCaveat(caveat)
		}
		return err
	}
	return bakery.FirstPartyCheckerFunc(f)
}

// ParseCaveat parses a caveat into an identifier,
// identifying the checker that should be used,
// and the argument to the checker (the rest of
// the string).
//
// The identifier is taken from all the characters
// before the first space character.
func ParseCaveat(cav string) (string, string, error) {
	if cav == "" {
		return "", "", fmt.Errorf("empty caveat")
	}
	i := strings.IndexByte(cav, ' ')
	if i <= 0 {
		return cav, "", nil
	}
	if i == 0 {
		return "", "", fmt.Errorf("caveat starts with space character")
	}
	return cav[0:i], cav[i+1:], nil
}
