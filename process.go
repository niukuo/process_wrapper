package wrapper

import (
	"context"
	"errors"
	"os/exec"
	"syscall"
	"time"
)

type Process interface {
	GetPid() int
	Renew(ctx context.Context, deadline time.Time) error
	Stop(ctx context.Context, force bool) error
	Done() <-chan struct{}
	Err() error
	GetStatus(ctx context.Context) (*Status, error)
}

type Status struct {
	Pid      int
	Deadline time.Time
	Stopping bool
}

type process struct {
	cmd *exec.Cmd

	cmdDoneCh <-chan error

	statusCh chan<- chan<- Status
	stopCh   chan<- bool
	doneCh   <-chan struct{}
	leaseCh  chan<- time.Time

	err error
}

func StartProcess(
	deadline time.Time,
	name string,
	args ...string,
) (Process, error) {

	cmd := exec.Command(name, args...)
	setCmdParams(cmd)

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	logger.Info("subprocess started",
		", pid: ", cmd.Process.Pid,
	)

	cmdDoneCh := make(chan error, 1)
	go func() { cmdDoneCh <- cmd.Wait() }()

	statusCh := make(chan chan<- Status)
	leaseCh := make(chan time.Time)
	stopCh := make(chan bool)
	doneCh := make(chan struct{})

	p := &process{
		cmd:       cmd,
		cmdDoneCh: cmdDoneCh,
		stopCh:    stopCh,
		doneCh:    doneCh,
		leaseCh:   leaseCh,
	}

	go func() {
		err := p.Run(deadline, leaseCh, statusCh, stopCh)

		logger.Info("wrapper stopped",
			", err: ", err,
		)

		p.err = err

		close(doneCh)
	}()

	return p, nil

}

func (p *process) GetPid() int {
	return p.cmd.Process.Pid
}

func (p *process) Renew(ctx context.Context, deadline time.Time) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-p.doneCh:
		return errors.New("stopped")
	case p.leaseCh <- deadline:
		return nil
	}
}

func (p *process) Stop(ctx context.Context, force bool) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-p.doneCh:
		return errors.New("stopped")
	case p.stopCh <- force:
		return nil
	}
}

func (p *process) Done() <-chan struct{} {

	return p.doneCh
}

func (p *process) Err() error {
	return p.err
}

func (p *process) GetStatus(ctx context.Context) (*Status, error) {

	ch := make(chan Status, 1)

	select {
	case p.statusCh <- ch:
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-p.doneCh:
		return nil, errors.New("stopped")
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-p.doneCh:
		return nil, errors.New("stopped")
	case s := <-ch:
		return &s, nil
	}

}

func (p *process) Run(
	deadline time.Time,
	leaseCh <-chan time.Time,
	statusCh <-chan chan<- Status,
	stopCh <-chan bool,
) (retErr error) {

	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer func() { cancel() }()

	stopping := false

	for {

		select {
		case deadline = <-leaseCh:
			cancel()
			ctx, cancel = context.WithDeadline(context.Background(), deadline)

		case <-ctx.Done():
			logger.Warning("lease timeout, killing process",
				", pid: ", p.cmd.Process.Pid,
			)
			leaseCh = nil
			stopping = true
			ctx, cancel = context.WithCancel(context.Background())
			p.cmd.Process.Kill()
			retErr = ctx.Err()

		case f := <-stopCh:
			stopping = true
			if f {
				p.cmd.Process.Kill()
			} else {
				p.cmd.Process.Signal(syscall.SIGTERM)
			}

		case ch := <-statusCh:
			d, _ := ctx.Deadline()
			ch <- Status{
				Pid:      p.cmd.Process.Pid,
				Stopping: stopping,
				Deadline: d,
			}

		case e := <-p.cmdDoneCh:
			if !stopping {
				logger.Warning("process exited unexpectedly",
					", pid: ", p.cmd.Process.Pid,
					", err: ", e,
				)
				retErr = e
			} else {
				logger.Info("process stopped",
					", pid: ", p.cmd.Process.Pid,
					", err: ", e,
				)
			}
			return
		}
	}

}
