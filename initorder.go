package main

import (
	"fmt"
	"sync"

	"github.com/golang/glog"
)

type InitHandler struct {
	Priority uint
	Name     string
	Do       func() error
	Redo     func() error
}

type Initializer struct {
	handlers []*InitHandler
	o        sync.Once
}

func (i *Initializer) init() {
	i.o.Do(func() {
		i.handlers = make([]*InitHandler, 100, 100)
	})
}

func (i *Initializer) Add(handler *InitHandler) {
	i.init()

	prio := handler.Priority
	if prio > uint(len(i.handlers)) {
		panic(fmt.Errorf("invalid init prio %d on %s", prio, handler.Name))
	}
	if existing := i.handlers[prio]; existing != nil {
		panic(fmt.Errorf("init prio %d already claimed by %s (attempt to usurp by %s)", prio, existing.Name, handler.Name))
	}
	i.handlers[prio] = handler
}

func (i *Initializer) Do() error {
	i.init()

	for _, v := range i.handlers {
		if v == nil {
			continue
		}
		if v.Do != nil {
			glog.Infof("[INIT] %d:%s", v.Priority, v.Name)
			err := v.Do()
			if err != nil {
				return fmt.Errorf("init: error executing %d:%s: %v", v.Priority, v.Name, err)
			}
		}
	}
	return nil
}

func (i *Initializer) Redo() error {
	i.init()

	for _, v := range i.handlers {
		if v == nil {
			continue
		}
		if v.Redo != nil {
			glog.Infof("[RELOAD] %d:%s", v.Priority, v.Name)
			err := v.Redo()
			if err != nil {
				return fmt.Errorf("init: error executing %d:%s: %v", v.Priority, v.Name, err)
			}
		}
	}
	return nil
}
