package worker

import (
	"fmt"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

var (
	once      sync.Once
	singleton *monitor
)

type description struct {
	active    atomic.Bool
	timeOutCb func()
}

type monitor struct {
	workers sync.Map
}

func getMonitor() *monitor {
	once.Do(func() {
		singleton = &monitor{
			workers: sync.Map{},
		}
	})

	return singleton
}

func (m *monitor) register(workerName string, onTimeOutCb func()) error {
	if _, ok := m.workers.Load(workerName); ok {
		return fmt.Errorf("worker %s already registered", workerName)
	}

	wd := description{
		timeOutCb: onTimeOutCb,
	}
	wd.active.Store(true)

	m.workers.Store(workerName, &wd)

	return nil
}

func (m *monitor) report(workerName string) {
	w, ok := m.workers.Load(workerName)
	if !ok {
		return
	}
	if work, ok := w.(*description); ok {
		work.active.Store(true)
	}
}

func (m *monitor) run() {
	ticker := time.NewTicker(time.Second * 5)
	defer ticker.Stop()

	signChan := make(chan os.Signal, 1)
	signal.Notify(signChan, syscall.SIGINT, syscall.SIGKILL, syscall.SIGKILL, syscall.SIGTERM, syscall.SIGQUIT)

	for {
		select {
		case <-signChan:
			return
		case <-ticker.C:
			m.workers.Range(func(_, value interface{}) bool {
				wd := value.(*description)
				if !wd.active.Load() {
					wd.timeOutCb()
				}
				wd.active.Store(false)
				return true
			})
		}
	}
}

func init() {
	go func() {
		getMonitor().run()
	}()
}
