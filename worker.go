package worker

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

const defCapacity = 50000

const (
	revBatchMsgMaxWait    = time.Millisecond * 5
	loopEventProcInterval = time.Millisecond * 10
)

type msg struct {
	id   int
	args []interface{}
}

type Worker struct {
	name       string
	fetchOnce  int
	router     *Router
	chSize     int
	loopFunc   func()
	beforeLoop func()
	afterLoop  func()
	stopped    atomic.Bool
	ch         chan *msg
	wg         sync.WaitGroup
	logger     ILogger
}

func NewWorker(logger ILogger, opts ...Option) (*Worker, error) {
	worker := &Worker{logger: logger}

	for _, opt := range opts {
		opt(worker)
	}

	return worker, worker.init()
}

func (w *Worker) init() error {
	if nil == w.router {
		return errors.New(fmt.Sprintf("worker %s router is nil", w.name))
	}

	w.router.name = w.name

	if nil == w.loopFunc {
		return errors.New(fmt.Sprintf("worker %s loop func is nil", w.name))
	}

	if 0 >= w.chSize {
		w.chSize = defCapacity
		w.logger.LogWarn("worker %s never set ch size. change to defCapacity %d", w.name, defCapacity)
	}

	w.ch = make(chan *msg, w.chSize)
	w.fetchOnce = w.chSize / 10

	w.logger.LogInfo("worker %s init success", w.name)
	return nil
}

func (w *Worker) PostMsg(id int, args ...interface{}) {
	if w.stopped.Load() {
		return
	}
	w.ch <- &msg{
		id:   id,
		args: args,
	}
}

func (w *Worker) GoStart() error {
	if nil == w.router {
		return errors.New(fmt.Sprintf("worker %s start without any router", w.name))
	}

	err := getMonitor().register(w.name, func() {
		var errStr = fmt.Sprintf("worker: %s may offline.", w.name)
		if w.router != nil {
			errStr = fmt.Sprintf("%s%s", errStr, w.router.curMsgInfo())
		}
		w.logger.LogError(errStr)
	})
	if nil != err {
		w.logger.LogError("register worker %s to monitor failed error: %v", w.name, err)
		return err
	}
	w.wg.Add(1)

	w.logger.LogInfo("worker %s GoStart", w.name)

	go func() {
		doLoopFuncTk := time.NewTicker(loopEventProcInterval)

		defer func() {
			w.wg.Done()
			defer doLoopFuncTk.Stop()

			w.logger.LogInfo("worker %s had exit", w.name)
		}()

		if nil != w.beforeLoop {
			w.beforeLoop()
		}

	EndLoop:
		for {
			select {
			case rec, ok := <-w.ch:
				if !ok {
					break EndLoop
				}
				if w.loop([]*msg{rec}) {
					break EndLoop
				}
			case <-doLoopFuncTk.C:
				if w.loop(nil) {
					break EndLoop
				}
			}
		}

		if nil != w.afterLoop {
			w.afterLoop()
		}
	}()

	return nil
}

func (w *Worker) Close() error {
	w.stopped.Store(true)
	close(w.ch)
	w.wg.Wait()

	w.logger.LogInfo("worker %s close done", w.name)
	return nil
}

func (w *Worker) GetRouter() *Router {
	return w.router
}

func (w *Worker) fetchMore(list []*msg) ([]*msg, bool) {
	t := time.Now()
	for {
		select {
		case rec, ok := <-w.ch:
			if !ok {
				return list, true
			}
			list = append(list, rec)
			if len(list) >= w.fetchOnce {
				return list, false
			}
			if since := time.Since(t); since > revBatchMsgMaxWait {
				return list, false
			}
		default:
			return list, false
		}
	}
}

func (w *Worker) loop(list []*msg) (exit bool) {
	getMonitor().report(w.name)

	list, exit = w.fetchMore(list)

	w.router.process(list)

	w.loopFunc()

	return
}
