package gotimeout

import (
	"time"
)

type EphemeralStoreValue interface{}
type EphemeralKeyValueStore struct {
	expirator *Expirator
	values    map[string]EphemeralStoreValue
}

type ephemeralExpirationProxy ExpirableID

func (e ephemeralExpirationProxy) ExpirationID() ExpirableID {
	return ExpirableID(e)
}

func NewEphemeralKeyValueStore() *EphemeralKeyValueStore {
	v := &EphemeralKeyValueStore{
		values: make(map[string]EphemeralStoreValue),
	}

	v.expirator = NewExpirator("", v)
	return v
}

func (e *EphemeralKeyValueStore) Put(k string, v EphemeralStoreValue, lifespan time.Duration) {
	e.values[k] = v
	e.expirator.ExpireObject(ephemeralExpirationProxy(k), lifespan)
}

func (e *EphemeralKeyValueStore) Get(k string) (v EphemeralStoreValue, ok bool) {
	v, ok = e.values[k]
	return
}

func (e *EphemeralKeyValueStore) Delete(k string) {
	e.expirator.CancelObjectExpiration(ephemeralExpirationProxy(k))
	delete(e.values, k)
}

func (e *EphemeralKeyValueStore) GetExpirable(id ExpirableID) (Expirable, error) {
	_, ok := e.values[string(id)]
	if !ok {
		return nil, nil
	}
	return ephemeralExpirationProxy(id), nil
}

func (e *EphemeralKeyValueStore) DestroyExpirable(ex Expirable) {
	delete(e.values, string(ex.ExpirationID()))
}
