package worker

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

const DefaultSlowTime = 20 * time.Millisecond

type MsgHdlType func(args ...interface{})

type Router struct {
	m  []MsgHdlType
	mu sync.RWMutex

	slowTime time.Duration

	name   string
	logger ILogger

	curMsg atomic.Pointer[msg]
}

func NewRouter(logger ILogger, maxId int, slow time.Duration) *Router {
	if slow == 0 {
		slow = DefaultSlowTime
	}
	return &Router{
		logger:   logger,
		m:        make([]MsgHdlType, maxId),
		slowTime: slow,
	}
}

func (r *Router) Register(id int, cb MsgHdlType) {
	if cb == nil {
		r.logger.LogError("worker[%s] router callback is nil, id=%v", r.name, id)
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if id >= len(r.m) {
		r.logger.LogError("worker[%s] router id out of range, id=%v", r.name, id)
		return
	}

	if r.m[id] != nil {
		r.logger.LogError("worker[%s] router callback is already registered, id=%v", r.name, id)
		return
	}
	r.m[id] = cb
}

func (r *Router) process(list []*msg) {
	for _, line := range list {
		t := time.Now()
		r.processMsg(line)
		if since := time.Since(t); since > r.slowTime {
			r.logger.LogDebug("process msg end! id:%v, cost:%v", line.id, since)
		}
	}
}

func (r *Router) processMsg(m *msg) {
	defer func() {
		if err := recover(); err != nil {
			r.logger.LogStack("panic error: %s", err)
		}
	}()
	r.mu.RLock()

	id := m.id
	if id >= len(r.m) {
		r.logger.LogError("worker[%s] router id out of range, id=%v", r.name, id)
		r.mu.RUnlock()
		return
	}

	callback := r.m[id]
	r.mu.RUnlock()

	if callback == nil {
		r.logger.LogError("worker[%s] router callback is nil, id=%v", r.name, id)
		return
	}

	r.curMsg.Store(m)

	callback(m.args[:]...)

	r.curMsg.Store(nil)
}

func (r *Router) curMsgInfo() string {
	m := r.curMsg.Load()
	if m == nil {
		return ""
	}
	return fmt.Sprintf("worker[%s] msg:{id:%d, args:%v}.", r.name, m.id, m.args)
}
