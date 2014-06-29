package macaroon_test

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/rogpeppe/macaroon"
	gc "gopkg.in/check.v1"
	_ "net/http"
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type macaroonSuite struct{}

var _ = gc.Suite(&macaroonSuite{})

func never(string) error {
	return fmt.Errorf("condition is never true")
}

func (*macaroonSuite) TestNoCaveats(c *gc.C) {
	rootKey := []byte("secret")
	m := macaroon.New(rootKey, "some id", "a location")
	c.Assert(m.Location(), gc.Equals, "a location")
	c.Assert(string(m.Id()), gc.Equals, "some id")

	err := m.Verify(rootKey, never, nil)
	c.Assert(err, gc.IsNil)
}

func (*macaroonSuite) TestFirstPartyCaveat(c *gc.C) {
	rootKey := []byte("secret")
	m := macaroon.New(rootKey, "some id", "a location")

	caveats := map[string]bool{
		"a caveat":       true,
		"another caveat": true,
	}
	tested := make(map[string]bool)

	for cav := range caveats {
		m.AddFirstPartyCaveat(cav)
	}
	expectErr := fmt.Errorf("condition not met")
	check := func(cav string) error {
		tested[cav] = true
		if caveats[cav] {
			return nil
		}
		return expectErr
	}
	err := m.Verify(rootKey, check, nil)
	c.Assert(err, gc.IsNil)

	c.Assert(tested, gc.DeepEquals, caveats)

	m.AddFirstPartyCaveat("not met")
	err = m.Verify(rootKey, check, nil)
	c.Assert(err, gc.Equals, expectErr)

	c.Assert(tested["not met"], gc.Equals, true)
}

func (*macaroonSuite) TestThirdPartyCaveat(c *gc.C) {
	rootKey := []byte("secret")
	m := macaroon.New(rootKey, "some id", "a location")

	dischargeRootKey := []byte("shared root key")
	thirdPartyCaveatId := "3rd party caveat"
	err := m.AddThirdPartyCaveat(dischargeRootKey, thirdPartyCaveatId, "remote.com")
	c.Assert(err, gc.IsNil)

	dm := macaroon.New(dischargeRootKey, thirdPartyCaveatId, "remote location")
	dm.Bind(m.Signature())
	err = m.Verify(rootKey, never, []*macaroon.Macaroon{dm})
	c.Assert(err, gc.IsNil)
}

type caveat struct {
	rootKey   string
	location  string
	condition string
}

type macaroonSpec struct {
	rootKey  string
	id       string
	caveats  []caveat
	location string
}

type conditionTest struct {
	conditions map[string]bool
	expectErr  string
}

var verifyTests = []struct {
	about      string
	macaroons  []macaroonSpec
	conditions []conditionTest
}{{
	about: "single third party caveat without discharge",
	macaroons: []macaroonSpec{{
		rootKey: "root-key",
		id:      "root-id",
		caveats: []caveat{{
			condition: "wonderful",
		}, {
			condition: "bob-is-great",
			location:  "bob",
			rootKey:   "bob-caveat-root-key",
		}},
	}},
	conditions: []conditionTest{{
		conditions: map[string]bool{
			"wonderful": true,
		},
		expectErr: `cannot find discharge macaroon for caveat "bob-is-great"`,
	}},
}, {
	about: "single third party caveat with discharge",
	macaroons: []macaroonSpec{{
		rootKey: "root-key",
		id:      "root-id",
		caveats: []caveat{{
			condition: "wonderful",
		}, {
			condition: "bob-is-great",
			location:  "bob",
			rootKey:   "bob-caveat-root-key",
		}},
	}, {
		location: "bob",
		rootKey:  "bob-caveat-root-key",
		id:       "bob-is-great",
	}},
	conditions: []conditionTest{{
		conditions: map[string]bool{
			"wonderful": true,
		},
	}, {
		conditions: map[string]bool{
			"wonderful": false,
		},
		expectErr: `condition "wonderful" not met`,
	}},
}, {
	about: "single third party caveat with discharge with mismatching root key",
	macaroons: []macaroonSpec{{
		rootKey: "root-key",
		id:      "root-id",
		caveats: []caveat{{
			condition: "wonderful",
		}, {
			condition: "bob-is-great",
			location:  "bob",
			rootKey:   "bob-caveat-root-key",
		}},
	}, {
		location: "bob",
		rootKey:  "bob-caveat-root-key-wrong",
		id:       "bob-is-great",
	}},
	conditions: []conditionTest{{
		conditions: map[string]bool{
			"wonderful": true,
		},
		expectErr: `signature mismatch after caveat verification`,
	}},
}, {
	about: "single third party caveat with two discharges",
	macaroons: []macaroonSpec{{
		rootKey: "root-key",
		id:      "root-id",
		caveats: []caveat{{
			condition: "wonderful",
		}, {
			condition: "bob-is-great",
			location:  "bob",
			rootKey:   "bob-caveat-root-key",
		}},
	}, {
		location: "bob",
		rootKey:  "bob-caveat-root-key",
		id:       "bob-is-great",
		caveats: []caveat{{
			condition: "splendid",
		}},
	}, {
		location: "bob",
		rootKey:  "bob-caveat-root-key",
		id:       "bob-is-great",
		caveats: []caveat{{
			condition: "top of the world",
		}},
	}},
	conditions: []conditionTest{{
		conditions: map[string]bool{
			"wonderful": true,
		},
		expectErr: `condition "top of the world" not met`,
	}, {
		conditions: map[string]bool{
			"wonderful":        true,
			"splendid":         true,
			"top of the world": true,
		},
	}, {
		conditions: map[string]bool{
			"wonderful":        true,
			"splendid":         false,
			"top of the world": true,
		},
	}, {
		conditions: map[string]bool{
			"wonderful":        true,
			"splendid":         true,
			"top of the world": false,
		},
	}},
}, {
	about: "two third party caveats",
	macaroons: []macaroonSpec{{
		rootKey: "root-key",
		id:      "root-id",
		caveats: []caveat{{
			condition: "wonderful",
		}, {
			condition: "bob-is-great",
			location:  "bob",
			rootKey:   "bob-caveat-root-key",
		}, {
			condition: "charlie-is-great",
			location:  "charlie",
			rootKey:   "charlie-caveat-root-key",
		}},
	}, {
		location: "bob",
		rootKey:  "bob-caveat-root-key",
		id:       "bob-is-great",
		caveats: []caveat{{
			condition: "splendid",
		}},
	}, {
		location: "charlie",
		rootKey:  "charlie-caveat-root-key",
		id:       "charlie-is-great",
		caveats: []caveat{{
			condition: "top of the world",
		}},
	}},
	conditions: []conditionTest{{
		conditions: map[string]bool{
			"wonderful":        true,
			"splendid":         true,
			"top of the world": true,
		},
	}, {
		conditions: map[string]bool{
			"wonderful":        true,
			"splendid":         false,
			"top of the world": true,
		},
		expectErr: `condition "splendid" not met`,
	}, {
		conditions: map[string]bool{
			"wonderful":        true,
			"splendid":         true,
			"top of the world": false,
		},
		expectErr: `condition "top of the world" not met`,
	}},
}, {
	about: "third party caveat with undischarged third party caveat",
	macaroons: []macaroonSpec{{
		rootKey: "root-key",
		id:      "root-id",
		caveats: []caveat{{
			condition: "wonderful",
		}, {
			condition: "bob-is-great",
			location:  "bob",
			rootKey:   "bob-caveat-root-key",
		}},
	}, {
		location: "bob",
		rootKey:  "bob-caveat-root-key",
		id:       "bob-is-great",
		caveats: []caveat{{
			condition: "splendid",
		}, {
			condition: "barbara-is-great",
			location:  "barbara",
			rootKey:   "barbara-caveat-root-key",
		}},
	}},
	conditions: []conditionTest{{
		conditions: map[string]bool{
			"wonderful": true,
			"splendid":  true,
		},
		expectErr: `cannot find discharge macaroon for caveat "barbara-is-great"`,
	}},
}, {
	about: "recursive third party caveats",
	macaroons: []macaroonSpec{{
		rootKey: "root-key",
		id:      "root-id",
		caveats: []caveat{{
			condition: "wonderful",
		}, {
			condition: "bob-is-great",
			location:  "bob",
			rootKey:   "bob-caveat-root-key",
		}, {
			condition: "charlie-is-great",
			location:  "chjarlie",
			rootKey:   "charlie-caveat-root-key",
		}},
	}, {
		location: "bob",
		rootKey:  "bob-caveat-root-key",
		id:       "bob-is-great",
		caveats: []caveat{{
			condition: "splendid",
		}, {
			condition: "barbara-is-great",
			location:  "barbara",
			rootKey:   "barbara-caveat-root-key",
		}},
	}, {
		location: "charlie",
		rootKey:  "charlie-caveat-root-key",
		id:       "charlie-is-great",
		caveats: []caveat{{
			condition: "splendid",
		}, {
			condition: "celine-is-great",
			location:  "celine",
			rootKey:   "celine-caveat-root-key",
		}},
	}, {
		location: "barbara",
		rootKey:  "barbara-caveat-root-key",
		id:       "barbara-is-great",
		caveats: []caveat{{
			condition: "spiffing",
		}, {
			condition: "ben-is-great",
			location:  "ben",
			rootKey:   "ben-caveat-root-key",
		}},
	}, {
		location: "ben",
		rootKey:  "ben-caveat-root-key",
		id:       "ben-is-great",
	}, {
		location: "celine",
		rootKey:  "celine-caveat-root-key",
		id:       "celine-is-great",
		caveats: []caveat{{
			condition: "high-fiving",
		}},
	}},
	conditions: []conditionTest{{
		conditions: map[string]bool{
			"wonderful":   true,
			"splendid":    true,
			"high-fiving": true,
			"spiffing":    true,
		},
	}, {
		conditions: map[string]bool{
			"wonderful":   true,
			"splendid":    true,
			"high-fiving": false,
			"spiffing":    true,
		},
		expectErr: `condition "high-fiving" not met`,
	}},
}}

func (*macaroonSuite) TestVerify(c *gc.C) {
	for i, test := range verifyTests {
		c.Logf("test %d: %s", i, test.about)
		var macaroons []*macaroon.Macaroon
		for _, mspec := range test.macaroons {
			m := macaroon.New([]byte(mspec.rootKey), mspec.id, mspec.location)
			for _, cav := range mspec.caveats {
				if cav.location != "" {
					err := m.AddThirdPartyCaveat([]byte(cav.rootKey), cav.condition, cav.location)
					c.Assert(err, gc.IsNil)
				} else {
					m.AddFirstPartyCaveat(cav.condition)
				}
			}
			macaroons = append(macaroons, m)
		}
		primaryMac := macaroons[0]
		discharges := macaroons[1:]
		for _, m := range discharges {
			m.Bind(primaryMac.Signature())
		}
		for _, cond := range test.conditions {
			c.Logf("conditions %#v", cond.conditions)
			check := func(cav string) error {
				if cond.conditions[cav] {
					return nil
				}
				return fmt.Errorf("condition %q not met", cav)
			}
			err := primaryMac.Verify(
				[]byte(test.macaroons[0].rootKey),
				check,
				discharges,
			)
			if cond.expectErr != "" {
				c.Assert(err, gc.ErrorMatches, cond.expectErr)
			} else {
				c.Assert(err, gc.IsNil)
			}
		}
	}
}

func (*macaroonSuite) TestMarshalJSON(c *gc.C) {
	rootKey := []byte("secret")
	m0 := macaroon.New(rootKey, "some id", "a location")
	m0.AddFirstPartyCaveat("account = 3735928559")
	m0JSON, err := json.Marshal(m0)
	c.Assert(err, gc.IsNil)
	var m1 macaroon.Macaroon
	err = json.Unmarshal(m0JSON, &m1)
	c.Assert(err, gc.IsNil)
	c.Assert(m0.Location(), gc.Equals, m1.Location())
	c.Assert(m0.Id(), gc.Equals, m1.Id())
	c.Assert(
		hex.EncodeToString(m0.Signature()),
		gc.Equals,
		hex.EncodeToString(m1.Signature()))
}

func (*macaroonSuite) TestUnmarshalJSON(c *gc.C) {
	var original, got macaroon.Macaroon
	jsonData := `{"caveats":[{"cid":"account = 3735928559"},{"cid":"time < 2015-01-01T00:00"},{"cid":"email = alice@example.org"}],"location":"http:\\/\\/mybank\\/","identifier":"we used our secret key","signature":"882e6d59496ed5245edb7ab5b8839ecd63e5d504e54839804f164070d8eed952"}`
	mJSON := []byte(jsonData)
	err := json.Unmarshal(mJSON, &original)
	c.Assert(err, gc.IsNil)
	data, err := original.MarshalJSON()
	c.Assert(err, gc.IsNil)
	err = json.Unmarshal(data, &got)
	c.Assert(err, gc.IsNil)
	c.Assert(got, gc.DeepEquals, original)
	c.Assert(original.Signature(), gc.DeepEquals,
		[]byte{0x88, 0x2e, 0x6d, 0x59, 0x49, 0x6e, 0xd5, 0x24, 0x5e, 0xdb, 0x7a, 0xb5, 0xb8, 0x83,
			0x9e, 0xcd, 0x63, 0xe5, 0xd5, 0x04, 0xe5, 0x48, 0x39, 0x80, 0x4f, 0x16, 0x40, 0x70,
			0xd8, 0xee, 0xd9, 0x52})
}
