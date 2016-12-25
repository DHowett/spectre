package model

import "sync"

type Driver interface {
	// Open returns a new model.Provider. The arguments
	// slice is interpreted in a driver-specific manner.
	Open(arguments ...interface{}) (Provider, error)
}

var driverOnce sync.Once
var driverMutex sync.RWMutex
var drivers map[string]Driver

func Register(name string, driver Driver) {
	driverOnce.Do(func() {
		drivers = make(map[string]Driver)
	})
	driverMutex.Lock()
	defer driverMutex.Unlock()
	drivers[name] = driver
}

func getDriver(name string) Driver {
	driverMutex.RLock()
	defer driverMutex.RUnlock()
	return drivers[name]
}

func Open(driver string, arguments ...interface{}) (Provider, error) {
	d := getDriver(driver)
	if d == nil {
		return nil, ErrUnknownDriver
	}
	return d.Open(arguments...)
}
