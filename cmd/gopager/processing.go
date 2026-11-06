package main

import (
	"context"
	"log/slog"
	"sync"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/persistence/ent/processingstore"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/processing/worker"
)

// processingWorkerRuntime holds the dispatcher runner; Core regex is in-process and needs no host lifetime.
type processingWorkerRuntime struct {
	dispatcher platform.Runner
}

// dueListener delivers a signal whenever intake announces new due work.
type dueListener interface {
	Listen(ctx context.Context, channel string) (<-chan struct{}, error)
}

// wakeableRunner is a platform runner that can be told to look for work immediately.
type wakeableRunner interface {
	platform.Runner
	Wake()
}

// notifiedDispatcher runs a dispatcher and wakes it on every due-work notification, which
// makes the dispatcher's poll interval only a fallback for missed notifications.
type notifiedDispatcher struct {
	dispatcher wakeableRunner
	listener   dueListener
}

// Run listens for due-work notifications while the dispatcher runs and returns the dispatcher's
// result after the listener has ended.
func (n notifiedDispatcher) Run(ctx context.Context) error {
	listenContext, stopListening := context.WithCancel(ctx)
	signals, err := n.listener.Listen(listenContext, processing.DueNotificationChannel)
	if err != nil {
		stopListening()
		return err
	}
	var forwarder sync.WaitGroup
	forwarder.Go(func() {
		for range signals {
			n.dispatcher.Wake()
		}
	})
	err = n.dispatcher.Run(ctx)
	stopListening()
	forwarder.Wait()
	return err
}

// composeProcessingWorker registers real primary due selection and the explicit current source boundary.
func composeProcessingWorker(ctx context.Context, store *postgres.Store, source worker.WorkSourceOwner,
	publisher worker.Publisher, config worker.DispatchConfig) (*processingWorkerRuntime, error) {
	ctx, cancel := context.WithTimeout(ctx, platform.DefaultCallTimeout)
	defer cancel()
	repository, err := processingstore.New(store)
	if err != nil {
		return nil, err
	}
	service, err := worker.NewWithPublisher(store, repository, source, processing.CoreRegex{}, uuid.New(), publisher)
	if err != nil {
		return nil, err
	}
	dispatcher, err := worker.NewDispatcher(store, repository, service, slog.Default(), config)
	if err != nil {
		return nil, err
	}
	return &processingWorkerRuntime{dispatcher: notifiedDispatcher{dispatcher: dispatcher, listener: store}}, nil
}

// Close is retained for the composition lifecycle; the worker owns no compute host.
func (r *processingWorkerRuntime) Close(context.Context) error {
	return nil
}
