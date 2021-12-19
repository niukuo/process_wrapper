package wrapper

import (
	"fmt"
	"time"
)

type monitor struct {
	level       string
	description string
}

type MonitorOptions struct {
	Timeout    time.Duration
	TimeoutMsg string
	ReportFn   func(level, description string)
}

func MonitorWorker(
	options MonitorOptions,
	optCh <-chan MonitorOptions,
	monCh <-chan monitor,
	stopCh <-chan struct{},
) error {

	t := time.NewTimer(options.Timeout)
	defer t.Stop()

	for {
		select {
		case o := <-optCh:
			if o.ReportFn != nil {
				options.ReportFn = o.ReportFn
			}
			if o.Timeout != 0 {
				options.Timeout = o.Timeout
			}
			if o.TimeoutMsg != "" {
				options.TimeoutMsg = o.TimeoutMsg
			}

		case m := <-monCh:
			options.ReportFn(m.level, m.description)
			if !t.Stop() {
				<-t.C
			}
			t.Reset(options.Timeout)

		case <-t.C:
			logger.Warning("monitor timeout",
				", timeout: ", options.Timeout,
				", msg: ", options.TimeoutMsg,
			)
			options.ReportFn("error", fmt.Sprintf("monitor timeout: %s", options.TimeoutMsg))
			t.Reset(options.Timeout)

		case <-stopCh:
			return nil
		}
	}

}
