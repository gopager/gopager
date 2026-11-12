// Command webhook is the GoPager Delivery plugin that POSTs a notification as JSON to a URL,
// optionally with a Bearer token and an HMAC-SHA256 signature.
package main

import (
	"github.com/gopager/gopager/pluginsdk"
	"github.com/gopager/gopager/pluginsdk/safehttp"
)

func main() {
	pluginsdk.ServeDelivery(&webhook{newClient: safehttp.NewClient})
}
