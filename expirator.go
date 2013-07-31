package main

import (
	"encoding/gob"
	"github.com/golang/glog"
	"os"
	"path/filepath"
	"time"
)

type ExpirableID string

type ExpirationHandle struct {
	ExpirationTime  time.Time
	ID              ExpirableID
	expirationTimer *time.Timer
}

type Expirator struct {
	dataPath            string
	expirableStore      ExpirableStore
	expirationMap       map[ExpirableID]*ExpirationHandle
	expirationChannel   chan *ExpirationHandle
	flushRequired       bool
	urgentFlushRequired bool
}

type Expirable interface {
	ExpirationID() ExpirableID
}

type ExpirableStore interface {
	Get(ExpirableID) (Expirable, error)
	Destroy(Expirable)
}

func NewExpirator(path string, store ExpirableStore) *Expirator {
	return &Expirator{
		expirableStore:    store,
		dataPath:          filepath.Join(path, "expiry.gob"),
		expirationChannel: make(chan *ExpirationHandle, 1000),
	}
}

func (e *Expirator) loadExpirations() {
	file, err := os.Open(e.dataPath)
	if err != nil {
		return
	}

	gobDecoder := gob.NewDecoder(file)
	tempMap := make(map[ExpirableID]*ExpirationHandle)
	gobDecoder.Decode(&tempMap)
	file.Close()

	for _, v := range tempMap {
		e.registerExpirationHandle(v)
	}
	glog.Info("Finished loading expirations.")
}

func (e *Expirator) saveExpirations() {
	if e.expirationMap == nil {
		return
	}

	file, err := os.Create(e.dataPath)
	if err != nil {
		glog.Error("Error writing expiration data: ", err.Error())
		return
	}

	gobEncoder := gob.NewEncoder(file)
	gobEncoder.Encode(e.expirationMap)

	file.Close()

	e.flushRequired, e.urgentFlushRequired = false, false
}

func (e *Expirator) registerExpirationHandle(ex *ExpirationHandle) {
	expiryFunc := func() { e.expirationChannel <- ex }

	if e.expirationMap == nil {
		e.expirationMap = make(map[ExpirableID]*ExpirationHandle)
	}

	if ex.expirationTimer != nil {
		e.cancelExpirationHandle(ex)
		glog.Info("Existing expiration for ", ex.ID, " cancelled")
	}

	glog.Info("Registering expiration for ", ex.ID)
	now := time.Now()
	if ex.ExpirationTime.After(now) {
		e.expirationMap[ex.ID] = ex
		e.urgentFlushRequired = true

		ex.expirationTimer = time.AfterFunc(ex.ExpirationTime.Sub(now), expiryFunc)
	} else {
		glog.Warning("Force-expiring outdated handle ", ex.ID)
		expiryFunc()
	}
}

func (e *Expirator) cancelExpirationHandle(ex *ExpirationHandle) {
	ex.expirationTimer.Stop()
	delete(e.expirationMap, ex.ID)
	e.urgentFlushRequired = true

	glog.Info("Execution order belayed for ", ex.ID)
}

func (e *Expirator) Run() {
	go e.loadExpirations()
	glog.Info("Starting expirator.")
	flushTicker, urgentFlushTicker := time.NewTicker(30*time.Second), time.NewTicker(1*time.Second)
	for {
		select {
		// 30-second flush timer (only save if changed)
		case _ = <-flushTicker.C:
			if e.expirationMap != nil && (e.flushRequired || e.urgentFlushRequired) {
				glog.Info("Flushing paste expirations to disk.")
				e.saveExpirations()
			}
		// 1-second flush timer (only save if *super-urgent, but still throttle)
		case _ = <-urgentFlushTicker.C:
			if e.expirationMap != nil && e.urgentFlushRequired {
				glog.Info("Urgently flushing paste expirations to disk.")
				e.saveExpirations()
			}
		case expiration := <-e.expirationChannel:
			glog.Info("Expiring ", expiration.ID)
			expirable, _ := e.expirableStore.Get(expiration.ID)
			if expirable != nil {
				e.expirableStore.Destroy(expirable)
			}

			delete(e.expirationMap, expiration.ID)
			e.flushRequired = true
		}
	}
}

func (e *Expirator) ExpireObject(ex Expirable, dur time.Duration) {
	id := ex.ExpirationID()
	exh, ok := e.expirationMap[id]
	if !ok {
		exh = &ExpirationHandle{ID: id}
	}
	exh.ExpirationTime = time.Now().Add(dur)
	e.registerExpirationHandle(exh)
}

func (e *Expirator) CancelObjectExpiration(ex Expirable) {
	id := ex.ExpirationID()
	exh, ok := e.expirationMap[id]
	if ok {
		e.cancelExpirationHandle(exh)
	}
}

func (e *Expirator) ObjectHasExpiration(ex Expirable) bool {
	id := ex.ExpirationID()
	_, ok := e.expirationMap[id]
	return ok
}
