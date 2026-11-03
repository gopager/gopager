package main

import (
	"context"
	"errors"
	"log/slog"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/persistence/ent/processingstore"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/processing/worker"
)

// processingWorkerRuntime retains host lifetime until the platform runner has drained.
type processingWorkerRuntime struct {
	dispatcher *worker.Dispatcher
	host       *processing.WASMHost
}

// composeProcessingWorker registers real primary due selection and the explicit current source boundary.
func composeProcessingWorker(ctx context.Context, store *postgres.Store, source worker.WorkSourceOwner,
	config worker.DispatchConfig) (*processingWorkerRuntime, error) {
	ctx, cancel := context.WithTimeout(ctx, platform.DefaultCallTimeout)
	defer cancel()
	repository, err := processingstore.New(store)
	if err != nil {
		return nil, err
	}
	host, err := processing.NewWASMHost(ctx)
	if err != nil {
		return nil, err
	}
	service, err := worker.New(store, repository, source, host, uuid.New())
	if err != nil {
		return nil, errors.Join(err, host.Close(context.WithoutCancel(ctx)))
	}
	dispatcher, err := worker.NewDispatcher(store, repository, service, slog.Default(), config)
	if err != nil {
		return nil, errors.Join(err, host.Close(context.WithoutCancel(ctx)))
	}
	return &processingWorkerRuntime{dispatcher: dispatcher, host: host}, nil
}

// Close releases the compute host only after the caller has joined the platform runner.
func (r *processingWorkerRuntime) Close(ctx context.Context) error {
	return r.host.Close(context.WithoutCancel(ctx))
}
