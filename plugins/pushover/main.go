// Command pushover is the GoPager Delivery plugin that sends a notification through the
// Pushover messages API.
package main

import (
	"github.com/gopager/gopager/pluginsdk"
	"github.com/gopager/gopager/pluginsdk/safehttp"
)

func main() {
	pluginsdk.ServeDelivery(&pushover{newClient: safehttp.NewClient})
}
