package main

import (
	"encoding/json"
	"net/http"
	"sync"
)

type HealthServer struct {
	mtx             sync.Mutex
	metrics         map[string]interface{}
	computedMetrics map[string]func() interface{}
}

func (h *HealthServer) IncrementMetric(key string) {
	h.mtx.Lock()
	defer h.mtx.Unlock()
	if h.metrics == nil {
		h.metrics = make(map[string]interface{})
	}
	var val int
	if pV, ok := h.metrics[key]; ok {
		val, ok = pV.(int)
	}
	val++
	h.metrics[key] = val
}

func (h *HealthServer) SetMetric(key string, value interface{}) {
	h.mtx.Lock()
	defer h.mtx.Unlock()
	if h.metrics == nil {
		h.metrics = make(map[string]interface{})
	}
	h.metrics[key] = value
}

func (h *HealthServer) RegisterComputedMetric(key string, closure func() interface{}) {
	h.mtx.Lock()
	defer h.mtx.Unlock()
	if h.computedMetrics == nil {
		h.computedMetrics = make(map[string]func() interface{})
	}
	h.computedMetrics[key] = closure
}

func (h *HealthServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mtx.Lock()
	defer h.mtx.Unlock()
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)

	defer func() {
		if rec := recover(); rec != nil {
			if err, ok := rec.(error); ok {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(err.Error()))
			}
		}
	}()

	m := make(map[string]interface{})
	if h.metrics != nil {
		for k, v := range h.metrics {
			m[k] = v
		}
	}
	if h.computedMetrics != nil {
		for k, cl := range h.computedMetrics {
			m[k] = cl()
		}
	}

	w.WriteHeader(http.StatusOK)
	enc.Encode(m)
}

func (h *HealthServer) Run(addr string) {
	sm := http.NewServeMux()
	sm.Handle("/ok", h)
	http.ListenAndServe(addr, sm)
}
