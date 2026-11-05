package main

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/persistence/ent/processingstore"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/processing/worker"
)

// processingWorkerRuntime holds the dispatcher; Core regex is in-process and needs no host lifetime.
type processingWorkerRuntime struct {
	dispatcher *worker.Dispatcher
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
	return &processingWorkerRuntime{dispatcher: dispatcher}, nil
}

// Close is retained for the composition lifecycle; the worker owns no compute host.
func (r *processingWorkerRuntime) Close(context.Context) error {
	return nil
}
