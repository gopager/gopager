package main

import (
	"testing"

	"github.com/gopager/gopager/internal/processing"
)

// TestBindDeliveryUsesThePluginHost proves that binding makes the plugin host the target provider and
// the placement source, and that it refuses to run without a host or after the Delivery side
// was composed with another provider.
func TestBindDeliveryUsesThePluginHost(t *testing.T) {
	t.Parallel()
	host := &processing.DeliveryHost{}
	runtime := &processingAPIRuntime{deliveryHost: host}

	bound := &core{}
	if err := runtime.bindDelivery(bound); err != nil {
		t.Fatal(err)
	}
	targets, ok := bound.deliveryConfig.targets.(pluginDeliveryTargets)
	if !ok || targets.host != host || bound.deliveryConfig.placement == nil {
		t.Fatalf("delivery configuration not bound to the plugin host: %+v", bound.deliveryConfig)
	}

	if err := runtime.bindDelivery(&core{delivery: &deliveryRuntime{}}); err == nil {
		t.Fatal("binding after the Delivery side was composed was accepted")
	}
	if err := (&processingAPIRuntime{}).bindDelivery(&core{}); err == nil {
		t.Fatal("binding without a plugin host was accepted")
	}
	var absent *processingAPIRuntime
	if err := absent.bindDelivery(&core{}); err == nil {
		t.Fatal("binding a nil runtime was accepted")
	}
}
