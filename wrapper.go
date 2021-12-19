package wrapper

import (
	"context"
	"fmt"
	"time"
)

type Wrapper interface {
	StartOrRenew(ctx context.Context, deadline time.Time) error
	Stop(ctx context.Context, force bool) (Process, error)
	UpdateMonitorOptions(ctx context.Context, opts MonitorOptions) error
}

type wrapper struct {
	monCh     chan<- monitor
	monOptCh  chan<- MonitorOptions
	monDoneCh <-chan struct{}
	monErr    error

	procName string
	procArgs []string

	process Process
}

func StartWrapper(
	monopt MonitorOptions,
	name string,
	args []string,
) (Wrapper, error) {

	stopCh := make(chan struct{})

	monCh := make(chan monitor)
	monOptCh := make(chan MonitorOptions)
	monDoneCh := make(chan struct{})

	w := &wrapper{
		monCh:     monCh,
		monOptCh:  monOptCh,
		monDoneCh: monDoneCh,

		procName: name,
		procArgs: args,
	}

	go func() {
		w.monErr = MonitorWorker(MonitorOptions{
			Timeout:  30 * time.Second,
			ReportFn: func(level, description string) {},
		}, monOptCh, monCh, stopCh)
		close(monDoneCh)
	}()

	return w, nil
}

func (w *wrapper) StartOrRenew(ctx context.Context, deadline time.Time) error {
	if w.process == nil {
		process, err := StartProcess(deadline, w.procName, w.procArgs...)
		if err != nil {
			logger.Warning("start process failed",
				", name: ", w.procName,
				", args: ", w.procArgs,
				", err: ", err,
			)
			w.SetMonitor(ctx, "error", fmt.Errorf("start failed: %w", err).Error())
			return err
		}
		w.process = process
		logger.Info("process started",
			", pid: ", w.process.GetPid(),
			", name: ", w.procName,
			", args: ", w.procArgs,
		)
		w.SetMonitor(ctx, "good", fmt.Sprintf("started, pid: %d", process.GetPid()))
		return nil
	}

	if err := w.process.Renew(ctx, deadline); err != nil {
		logger.Warning("renew failed",
			", err: ", err,
		)
		w.SetMonitor(ctx, "error", fmt.Errorf("renew failed: %w", err).Error())
		return err
	}

	w.SetMonitor(ctx, "good", fmt.Sprintf("running, pid: %d", w.process.GetPid()))
	return nil
}

func (w *wrapper) Stop(ctx context.Context, force bool) (Process, error) {
	if w.process == nil {
		w.SetMonitor(ctx, "info", "not running")
		return nil, nil
	}

	logger.Warning("stopping process",
		", pid: ", w.process.GetPid(),
		", force: ", force,
	)

	if err := w.process.Stop(ctx, force); err != nil {
		logger.Warning("stop process failed",
			", pid: ", w.process.GetPid(),
			", err: ", err,
		)
		w.SetMonitor(ctx, "error", fmt.Errorf("stop failed: %w", err).Error())
		return nil, err
	}

	process := w.process
	w.process = nil
	w.SetMonitor(ctx, "info", fmt.Sprintf("stopping, pid: %d", process.GetPid()))

	return process, nil
}

func (w *wrapper) SetMonitor(ctx context.Context, level string, description string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-w.monDoneCh:
		return fmt.Errorf("stopped: %w", w.monErr)
	case w.monCh <- monitor{level: level, description: description}:
		return nil
	}
}

func (w *wrapper) UpdateMonitorOptions(ctx context.Context, opts MonitorOptions) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-w.monDoneCh:
		return fmt.Errorf("stopped: %w", w.monErr)
	case w.monOptCh <- opts:
		return nil
	}
}
