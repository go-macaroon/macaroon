package bakery

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
)

// Storage defines storage for macaroons.
// Calling its methods concurrently is allowed.
type Storage interface {
	// Put stores the item at the given location, overwriting
	// any item that might already be there.
	// TODO(rog) would it be better to lose the overwrite
	// semantics?
	Put(location string, item string) error

	// Get retrieves an item from the given location.
	// If the item is not there, it returns ErrNotFound.
	Get(location string) (item string, err error)

	// Del deletes the item from the given location.
	Del(location string) error
}

var ErrNotFound = errors.New("item not found")

// NewMemStorage returns an implementation of Storage
// that stores all items in memory.
func NewMemStorage() Storage {
	return &memStorage{
		values: make(map[string]string),
	}
}

type memStorage struct {
	mu     sync.Mutex
	values map[string]string
}

func (s memStorage) Put(location, item string) error {
	log.Printf("storage.Put[%q] %q", location, item)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[location] = item
	return nil
}

func (s memStorage) Get(location string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.values[location]
	if !ok {
		log.Printf("storage.Get[%q] -> not found", location)
		return "", ErrNotFound
	}
	log.Printf("storage.Get[%q] -> %q", location, item)
	return item, nil
}

func (s memStorage) Del(location string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.values, location)
	return nil
}

// storageItem is the format used to store items in
// the store.
type storageItem struct {
	Capability string
	RootKey    []byte
}

type storage struct {
	store Storage
}

func (s storage) Get(location string) (*storageItem, error) {
	itemStr, err := s.store.Get(location)
	if err != nil {
		return nil, err
	}
	var item storageItem
	if err := json.Unmarshal([]byte(itemStr), &item); err != nil {
		return nil, fmt.Errorf("badly formatted item in store: %v", err)
	}
	return &item, nil
}

func (s storage) Put(location string, item *storageItem) error {
	data, err := json.Marshal(item)
	if err != nil {
		panic(fmt.Errorf("cannot marshal storage item: %v", err))
	}
	return s.store.Put(location, string(data))
}
