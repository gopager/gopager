// Package plugintest drives a GoPager Delivery plugin exactly as the Core does, so plugin
// authors can test the real executable against local fakes of the provider they talk to.
//
// A Harness lays out a temporary plugin directory like the operator's, verifies the manifest with
// the Core's rules, starts the plugin through the Core's plugin host and runs attempts through the
// Core's Delivery host. The plugin therefore receives the same request the Core sends (merged
// configuration with Secrets resolved, Secret versions, typed inputs) and the test sees the same
// outcome the Core records, including the error that makes the Core record an attempt as
// uncertain.
//
// The package lives in the GoPager module and may be imported from other modules of the
// repository, such as the first-party plugins.
//
//	func TestDeliversToTheProvider(t *testing.T) {
//		provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
//			w.WriteHeader(http.StatusNoContent)
//		}))
//		defer provider.Close()
//
//		h := plugintest.New(t, plugintest.Options{
//			Executable:           plugintest.BuildPlugin(t, ".", "."),
//			ManifestTemplatePath: "manifest.template.json",
//		})
//		h.Configure(map[string]any{"url": provider.URL}, map[string]plugintest.Secret{
//			"bearerToken": plugintest.SecretValue("s3cret"),
//		})
//		outcome, err := h.Attempt(plugintest.AttemptInput{Body: "disk full"})
//		if err != nil || outcome.Status != processing.AttemptAccepted {
//			t.Fatalf("%+v %v", outcome, err)
//		}
//	}
package plugintest
