package bakery_test

import (
	"fmt"

	gc "gopkg.in/check.v1"

	"github.com/rogpeppe/macaroon"
	"github.com/rogpeppe/macaroon/bakery"
)

type DischargeSuite struct{}

var _ = gc.Suite(&DischargeSuite{})

func alwaysOK(string) error {
	return nil
}

func (*DischargeSuite) TestDischargeAllNoDischarges(c *gc.C) {
	rootKey := []byte("root key")
	m, err := macaroon.New(rootKey, "id0", "loc0")
	c.Assert(err, gc.IsNil)
	getDischarge := func(string, macaroon.Caveat) (*macaroon.Macaroon, error) {
		c.Errorf("getDischarge called unexpectedly")
		return nil, fmt.Errorf("nothing")
	}
	ms, err := bakery.DischargeAll(m, getDischarge)
	c.Assert(err, gc.IsNil)
	c.Assert(ms, gc.HasLen, 0)

	err = m.Verify(rootKey, alwaysOK, ms)
	c.Assert(err, gc.IsNil)
}

func (*DischargeSuite) TestDischargeAllManyDischarges(c *gc.C) {
	rootKey := []byte("root key")
	m0, err := macaroon.New(rootKey, "id0", "location0")
	c.Assert(err, gc.IsNil)
	totalRequired := 40
	id := 1
	addCaveats := func(m *macaroon.Macaroon) {
		for i := 0; i < 2; i++ {
			if totalRequired == 0 {
				break
			}
			cid := fmt.Sprint("id", id)
			err := m.AddThirdPartyCaveat([]byte("root key "+cid), cid, "somewhere")
			c.Assert(err, gc.IsNil)
			id++
			totalRequired--
		}
	}
	addCaveats(m0)
	getDischarge := func(loc string, cav macaroon.Caveat) (*macaroon.Macaroon, error) {
		c.Assert(loc, gc.Equals, "location0")
		m, err := macaroon.New([]byte("root key "+cav.Id), cav.Id, "")
		c.Assert(err, gc.IsNil)
		addCaveats(m)
		return m, nil
	}
	ms, err := bakery.DischargeAll(m0, getDischarge)
	c.Assert(err, gc.IsNil)
	c.Assert(ms, gc.HasLen, 40)

	for _, m := range ms {
		m.Bind(m0.Signature())
	}

	err = m0.Verify(rootKey, alwaysOK, ms)
	c.Assert(err, gc.IsNil)
}
