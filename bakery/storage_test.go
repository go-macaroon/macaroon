package bakery_test

import (
	"fmt"

	gc "gopkg.in/check.v1"

	"github.com/rogpeppe/macaroon/bakery"
)

type StorageSuite struct{}

var _ = gc.Suite(&StorageSuite{})

func (*StorageSuite) TestMemStorage(c *gc.C) {
	store := bakery.NewMemStorage()
	err := store.Put("foo", "bar")
	c.Assert(err, gc.IsNil)
	item, err := store.Get("foo")
	c.Assert(err, gc.IsNil)
	c.Assert(item, gc.Equals, "bar")

	err = store.Put("bletch", "blat")
	c.Assert(err, gc.IsNil)
	item, err = store.Get("bletch")
	c.Assert(err, gc.IsNil)
	c.Assert(item, gc.Equals, "blat")

	item, err = store.Get("nothing")
	c.Assert(err, gc.Equals, bakery.ErrNotFound)
	c.Assert(item, gc.Equals, "")

	err = store.Del("bletch")
	c.Assert(err, gc.IsNil)

	item, err = store.Get("bletch")
	c.Assert(err, gc.Equals, bakery.ErrNotFound)
	c.Assert(item, gc.Equals, "")
}

func (*StorageSuite) TestConcurrentMemStorage(c *gc.C) {
	// If locking is not done right, this test will
	// definitely trigger the race detector.
	done := make(chan struct{})
	store := bakery.NewMemStorage()
	for i := 0; i < 3; i++ {
		i := i
		go func() {
			k := fmt.Sprint(i)
			err := store.Put(k, k)
			c.Check(err, gc.IsNil)
			v, err := store.Get(k)
			c.Check(v, gc.Equals, k)
			err = store.Del(k)
			c.Check(err, gc.IsNil)
			done <- struct{}{}
		}()
	}
	for i := 0; i < 3; i++ {
		<-done
	}
}
