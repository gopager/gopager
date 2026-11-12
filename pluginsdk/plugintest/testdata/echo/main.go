// Command echo is the Delivery plugin the plugintest package tests itself with. The attempt body
// selects the behaviour: "echo" reports what the plugin received, "leak" puts a Secret into the
// receipt and "hang" never answers.
package main

import (
	"context"
	"encoding/json"
	"time"

	"github.com/gopager/gopager/pluginsdk"
)

type config struct {
	Title   string `json:"title"`
	Token   string `json:"token"`
	UserKey string `json:"userKey"`
}

// head returns the first three characters of a Secret, enough to tell values apart without
// echoing them.
func head(secret string) string {
	if len(secret) > 3 {
		return secret[:3]
	}
	return secret
}

func deliver(ctx context.Context, request pluginsdk.DeliveryRequest) pluginsdk.DeliveryResult {
	var cfg config
	if err := request.DecodeConfiguration(&cfg); err != nil {
		return pluginsdk.DeliveryResult{Status: pluginsdk.StatusPermanent, Code: "configuration-invalid"}
	}
	switch request.Attempt.Body {
	case "hang":
		<-ctx.Done()
		time.Sleep(time.Minute)
	case "leak":
		receipt, _ := json.Marshal(map[string]string{"token": cfg.Token})
		return pluginsdk.DeliveryResult{Status: pluginsdk.StatusAccepted, Code: "ok", Receipt: receipt}
	}
	if cfg.UserKey == "" {
		return pluginsdk.DeliveryResult{Status: pluginsdk.StatusPermanent, Code: "recipient-missing"}
	}
	priority, hasPriority := request.Attempt.Inputs["priority"]
	receipt, _ := json.Marshal(map[string]any{
		"title": cfg.Title, "tokenHead": head(cfg.Token), "userKeyHead": head(cfg.UserKey),
		"revision": request.InstanceRevision, "versions": request.SecretVersions,
		"subscriber": request.Attempt.SubscriberID, "outputKind": request.Attempt.OutputKind,
		"hasPriority": hasPriority, "priorityKind": priority.Kind,
	})
	return pluginsdk.DeliveryResult{Status: pluginsdk.StatusAccepted, Code: "ok", Receipt: receipt}
}

func main() {
	pluginsdk.ServeDelivery(pluginsdk.DeliveryHandlerFunc(deliver))
}
