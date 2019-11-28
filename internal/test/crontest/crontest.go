package crontest

import (
	"github.com/robfig/cron/v3"
	"sync/atomic"
)

// FakeCron implements the `CronScheduler' interface - but imediately runs all scheduled jobs.
// It is only intended for use in tests.
type FakeCron struct {
	counter int32
}

func (f FakeCron) AddFunc(spec string, job func()) (cron.EntryID, error) {
	defer func() {
		atomic.AddInt32(&f.counter, 1)
	}()
	job()
	return cron.EntryID(f.counter), nil
}

func (f FakeCron) Remove(id cron.EntryID) {}
