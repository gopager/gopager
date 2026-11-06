package pluginsdk_test

import (
	"testing"

	"github.com/gopager/gopager/pluginsdk"
)

func TestRequestMode(t *testing.T) {
	for _, test := range []struct {
		name, request, want string
		fails               bool
	}{
		{name: "explicit preview", request: `{"schemaVersion":2,"mode":"preview"}`, want: pluginsdk.ModePreview},
		{name: "explicit live", request: `{"schemaVersion":2,"mode":"live"}`, want: pluginsdk.ModeLive},
		{name: "absent defaults to live", request: `{"schemaVersion":2}`, want: pluginsdk.ModeLive},
		{name: "unknown mode", request: `{"mode":"dry"}`, fails: true},
		{name: "not JSON", request: `x`, fails: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := pluginsdk.RequestMode([]byte(test.request))
			if test.fails != (err != nil) || got != test.want {
				t.Fatalf("RequestMode = %q, %v; want %q, failure %v", got, err, test.want, test.fails)
			}
		})
	}
}
