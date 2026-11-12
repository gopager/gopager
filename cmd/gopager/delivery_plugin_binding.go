package main

import (
	"errors"

	"github.com/gopager/gopager/internal/platform"
)

// bindDelivery makes the plugin host the Delivery side's target provider and the worker runner's
// placement source. It must run before the Delivery side is first composed: afterwards the
// targets are already fixed, and the binding fails instead of silently keeping another provider.
func (runtime *processingAPIRuntime) bindDelivery(c *core) error {
	if runtime == nil || runtime.deliveryHost == nil {
		return platform.NewFailure(platform.CodeInvalidInput, errors.New("missing plugin delivery host"))
	}
	if c.delivery != nil {
		return platform.NewFailure(platform.CodeInvalidInput, errors.New("delivery side composed before plugin binding"))
	}
	c.deliveryConfig.targets = pluginDeliveryTargets{host: runtime.deliveryHost}
	c.deliveryConfig.placement = runtime.deliveryHost.Loadable
	return nil
}
