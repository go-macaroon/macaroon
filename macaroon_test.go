package macaroon_test

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"testing"

	gc "gopkg.in/check.v1"

	"gopkg.in/macaroon.v1"
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
	m := MustNew(rootKey, "some id", "a location")
	c.Assert(m.Location(), gc.Equals, "a location")
	c.Assert(m.Id(), gc.Equals, "some id")

	err := m.Verify(rootKey, never, nil)
	c.Assert(err, gc.IsNil)
}

func (*macaroonSuite) TestFirstPartyCaveat(c *gc.C) {
	rootKey := []byte("secret")
	m := MustNew(rootKey, "some id", "a location")

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
	m := MustNew(rootKey, "some id", "a location")

	dischargeRootKey := []byte("shared root key")
	thirdPartyCaveatId := "3rd party caveat"
	err := m.AddThirdPartyCaveat(dischargeRootKey, thirdPartyCaveatId, "remote.com")
	c.Assert(err, gc.IsNil)

	dm := MustNew(dischargeRootKey, thirdPartyCaveatId, "remote location")
	dm.Bind(m.Signature())
	err = m.Verify(rootKey, never, []*macaroon.Macaroon{dm})
	c.Assert(err, gc.IsNil)
}

func (*macaroonSuite) TestThirdPartyCaveatBadRandom(c *gc.C) {
	rootKey := []byte("secret")
	m := MustNew(rootKey, "some id", "a location")
	dischargeRootKey := []byte("shared root key")
	thirdPartyCaveatId := "3rd party caveat"

	err := macaroon.AddThirdPartyCaveatWithRand(m, dischargeRootKey, thirdPartyCaveatId, "remote.com", &macaroon.ErrorReader{})
	c.Assert(err, gc.ErrorMatches, "cannot generate random bytes: fail")
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
		expectErr: `condition "splendid" not met`,
	}, {
		conditions: map[string]bool{
			"wonderful":        true,
			"splendid":         true,
			"top of the world": true,
		},
		expectErr: `discharge macaroon "bob-is-great" was not used`,
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
		expectErr: `discharge macaroon "bob-is-great" was not used`,
	}},
}, {
	about: "one discharge used for two macaroons",
	macaroons: []macaroonSpec{{
		rootKey: "root-key",
		id:      "root-id",
		caveats: []caveat{{
			condition: "somewhere else",
			location:  "bob",
			rootKey:   "bob-caveat-root-key",
		}, {
			condition: "bob-is-great",
			location:  "charlie",
			rootKey:   "bob-caveat-root-key",
		}},
	}, {
		location: "bob",
		rootKey:  "bob-caveat-root-key",
		id:       "somewhere else",
		caveats: []caveat{{
			condition: "bob-is-great",
			location:  "charlie",
			rootKey:   "bob-caveat-root-key",
		}},
	}, {
		location: "bob",
		rootKey:  "bob-caveat-root-key",
		id:       "bob-is-great",
	}},
	conditions: []conditionTest{{
		expectErr: `discharge macaroon "bob-is-great" was used more than once`,
	}},
}, {
	about: "recursive third party caveat",
	macaroons: []macaroonSpec{{
		rootKey: "root-key",
		id:      "root-id",
		caveats: []caveat{{
			condition: "bob-is-great",
			location:  "bob",
			rootKey:   "bob-caveat-root-key",
		}},
	}, {
		location: "bob",
		rootKey:  "bob-caveat-root-key",
		id:       "bob-is-great",
		caveats: []caveat{{
			condition: "bob-is-great",
			location:  "charlie",
			rootKey:   "bob-caveat-root-key",
		}},
	}},
	conditions: []conditionTest{{
		expectErr: `discharge macaroon "bob-is-great" was used more than once`,
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
	about:     "recursive third party caveats",
	macaroons: recursiveThirdPartyCaveatMacaroons,
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
}, {
	about: "unused discharge",
	macaroons: []macaroonSpec{{
		rootKey: "root-key",
		id:      "root-id",
	}, {
		rootKey: "other-key",
		id:      "unused",
	}},
	conditions: []conditionTest{{
		expectErr: `discharge macaroon "unused" was not used`,
	}},
}}

var recursiveThirdPartyCaveatMacaroons = []macaroonSpec{{
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
}}

func (*macaroonSuite) TestVerify(c *gc.C) {
	for i, test := range verifyTests {
		c.Logf("test %d: %s", i, test.about)
		rootKey, primary, discharges := makeMacaroons(test.macaroons)
		for _, cond := range test.conditions {
			c.Logf("conditions %#v", cond.conditions)
			check := func(cav string) error {
				if cond.conditions[cav] {
					return nil
				}
				return fmt.Errorf("condition %q not met", cav)
			}
			err := primary.Verify(
				rootKey,
				check,
				discharges,
			)
			if cond.expectErr != "" {
				c.Assert(err, gc.ErrorMatches, cond.expectErr)
			} else {
				c.Assert(err, gc.IsNil)
			}

			// Cloned macaroon should have same verify result.
			cloneErr := primary.Clone().Verify(rootKey, check, discharges)
			c.Assert(cloneErr, gc.DeepEquals, err)
		}
	}
}

func (*macaroonSuite) TestMarshalJSON(c *gc.C) {
	rootKey := []byte("secret")
	m0 := MustNew(rootKey, "some id", "a location")
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

func (*macaroonSuite) TestJSONRoundTrip(c *gc.C) {
	// jsonData produced from the second example in libmacaroons
	// example README, but with the signature tweaked to
	// match our current behaviour.
	// TODO fix that behaviour so that our signatures match.
	jsonData := `{"caveats":[{"cid":"account = 3735928559"},{"cid":"this was how we remind auth of key\/pred","vid":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA027FAuBYhtHwJ58FX6UlVNFtFsGxQHS7uD\/w\/dedwv4Jjw7UorCREw5rXbRqIKhr","cl":"http:\/\/auth.mybank\/"}],"location":"http:\/\/mybank\/","identifier":"we used our other secret key","signature":"6e315b0b391e8c6cc6f8d88fc22933a13430fb289b2fb613cf70f746bbe7d27d"}`

	var m macaroon.Macaroon
	err := json.Unmarshal([]byte(jsonData), &m)
	c.Assert(err, gc.IsNil)
	c.Assert(hex.EncodeToString(m.Signature()), gc.Equals,
		"6e315b0b391e8c6cc6f8d88fc22933a13430fb289b2fb613cf70f746bbe7d27d")
	data, err := m.MarshalJSON()
	c.Assert(err, gc.IsNil)

	// Check that the round-tripped data is the same as the original
	// data when unmarshalled into an interface{}.
	var got interface{}
	err = json.Unmarshal(data, &got)
	c.Assert(err, gc.IsNil)

	var original interface{}
	err = json.Unmarshal([]byte(jsonData), &original)
	c.Assert(err, gc.IsNil)

	c.Assert(got, gc.DeepEquals, original)
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

func makeMacaroons(mspecs []macaroonSpec) (
	rootKey []byte,
	primary *macaroon.Macaroon,
	discharges []*macaroon.Macaroon,
) {
	var macaroons []*macaroon.Macaroon
	for _, mspec := range mspecs {
		macaroons = append(macaroons, makeMacaroon(mspec))
	}
	primary = macaroons[0]
	discharges = macaroons[1:]
	for _, m := range discharges {
		m.Bind(primary.Signature())
	}
	return []byte(mspecs[0].rootKey), primary, discharges
}

func makeMacaroon(mspec macaroonSpec) *macaroon.Macaroon {
	m := MustNew([]byte(mspec.rootKey), mspec.id, mspec.location)
	for _, cav := range mspec.caveats {
		if cav.location != "" {
			err := m.AddThirdPartyCaveat([]byte(cav.rootKey), cav.condition, cav.location)
			if err != nil {
				panic(err)
			}
		} else {
			m.AddFirstPartyCaveat(cav.condition)
		}
	}
	return m
}

func assertEqualMacaroons(c *gc.C, m0, m1 *macaroon.Macaroon) {
	m0json, err := m0.MarshalJSON()
	c.Assert(err, gc.IsNil)
	m1json, err := m1.MarshalJSON()
	var m0val, m1val interface{}
	err = json.Unmarshal(m0json, &m0val)
	c.Assert(err, gc.IsNil)
	err = json.Unmarshal(m1json, &m1val)
	c.Assert(err, gc.IsNil)
	c.Assert(m0val, gc.DeepEquals, m1val)
}

func (*macaroonSuite) TestBinaryRoundTrip(c *gc.C) {
	// Test the binary marshalling and unmarshalling of a macaroon with
	// first and third party caveats.
	rootKey := []byte("secret")
	m0 := MustNew(rootKey, "some id", "a location")
	err := m0.AddFirstPartyCaveat("first caveat")
	c.Assert(err, gc.IsNil)
	err = m0.AddFirstPartyCaveat("second caveat")
	c.Assert(err, gc.IsNil)
	err = m0.AddThirdPartyCaveat([]byte("shared root key"), "3rd party caveat", "remote.com")
	c.Assert(err, gc.IsNil)
	data, err := m0.MarshalBinary()
	c.Assert(err, gc.IsNil)
	var m1 macaroon.Macaroon
	err = m1.UnmarshalBinary(data)
	c.Assert(err, gc.IsNil)
	assertEqualMacaroons(c, m0, &m1)
}

func (*macaroonSuite) TestBinaryMarshalingAgainstLibmacaroon(c *gc.C) {
	// Test that a libmacaroon marshalled macaroon can be correctly unmarshaled
	data, err := base64.StdEncoding.DecodeString(
		"MDAxN2xvY2F0aW9uIHNvbWV3aGVyZQowMDEyaWRlbnRpZmllciBpZAowMDEzY2lkIGlkZW50aWZpZXIKMDA1MXZpZCAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAC4i9QwCgbL/wZGFvLQpsyhLOv0v6VjIo2KJv5miz+7krqCpt5EhmrL8pYO9xrhT80KMDAxM2NsIHRoaXJkIHBhcnR5CjAwMmZzaWduYXR1cmUg3BXkIDX0giAPPrgkDLbiMGYy/zsC2qPb4jU4G/dohkAK")
	c.Assert(err, gc.IsNil)
	var m0 macaroon.Macaroon
	err = m0.UnmarshalBinary(data)
	c.Assert(err, gc.IsNil)
	jsonData := []byte(`{"caveats":[{"cid":"identifier","vid":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAuIvUMAoGy/8GRhby0KbMoSzr9L+lYyKNiib+Zos/u5K6gqbeRIZqy/KWDvca4U/N","cl":"third party"}],"location":"somewhere","identifier":"id","signature":"dc15e42035f482200f3eb8240cb6e2306632ff3b02daa3dbe235381bf7688640"}`)
	var m1 macaroon.Macaroon
	err = m1.UnmarshalJSON(jsonData)
	c.Assert(err, gc.IsNil)
	assertEqualMacaroons(c, &m0, &m1)
}

func (*macaroonSuite) TestMacaroonFieldsTooBig(c *gc.C) {
	rootKey := []byte("secret")
	toobig := make([]byte, macaroon.MaxPacketLen)
	_, err := rand.Reader.Read(toobig)
	c.Assert(err, gc.IsNil)
	_, err = macaroon.New(rootKey, string(toobig), "a location")
	c.Assert(err, gc.ErrorMatches, "macaroon identifier too big")
	_, err = macaroon.New(rootKey, "some id", string(toobig))
	c.Assert(err, gc.ErrorMatches, "macaroon location too big")

	m0 := MustNew(rootKey, "some id", "a location")
	err = m0.AddThirdPartyCaveat([]byte("shared root key"), string(toobig), "remote.com")
	c.Assert(err, gc.ErrorMatches, "caveat identifier too big")
	err = m0.AddThirdPartyCaveat([]byte("shared root key"), "3rd party caveat", string(toobig))
	c.Assert(err, gc.ErrorMatches, "caveat location too big")
}
