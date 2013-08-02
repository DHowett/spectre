package main

import (
	"./expirator"
)

type AuthThrottleEntry struct {
	ID   string
	Hits int
}

type ExpiringAuthThrottleStore struct {
	throttles map[string]*AuthThrottleEntry
}

func NewExpiringAuthThrottleStore() *ExpiringAuthThrottleStore {
	return &ExpiringAuthThrottleStore{
		throttles: make(map[string]*AuthThrottleEntry),
	}
}

func (e *ExpiringAuthThrottleStore) Add(at *AuthThrottleEntry) {
	e.throttles[at.ID] = at
}

func (e *ExpiringAuthThrottleStore) Get(id expirator.ExpirableID) (expirator.Expirable, error) {
	v, ok := e.throttles[string(id)]
	if !ok {
		return nil, nil
	}
	return v, nil
}

func (e *ExpiringAuthThrottleStore) Destroy(ex expirator.Expirable) {
	delete(e.throttles, string(ex.ExpirationID()))
}

func (a *AuthThrottleEntry) ExpirationID() expirator.ExpirableID {
	return expirator.ExpirableID(a.ID)
}
