package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

var errInterrupted = errors.New("preparation interrupted")
var errTerminated = errors.New("preparation terminated")

func commandContext() (context.Context, func()) {
	ctx, cancel := context.WithCancelCause(context.Background())
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	done := make(chan struct{})
	exited := make(chan struct{})
	go func() {
		defer close(exited)
		select {
		case received := <-signals:
			if received == syscall.SIGTERM {
				cancel(errTerminated)
			} else {
				cancel(errInterrupted)
			}
		case <-done:
		}
	}()
	var once sync.Once
	return ctx, func() { once.Do(func() { signal.Stop(signals); close(done); <-exited; cancel(context.Canceled) }) }
}
