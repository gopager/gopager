package main

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gopager/gopager/internal/platform"
	testpostgres "github.com/gopager/gopager/internal/testutil/postgres"
)

// TestRunProcessHelper invokes the actual command entry in an isolated test process.
func TestRunProcessHelper(t *testing.T) { //nolint:paralleltest // child-process entry point that replaces os.Args and exits the process
	roles := os.Getenv("GOPAGER_TEST_PROCESS_HELPER")
	if roles == "" {
		return
	}
	os.Args = []string{"gopager", "--profile=full", "--roles=" + roles,
		"--migration-mode=apply", "--listen-address=" + os.Getenv("GOPAGER_TEST_LISTEN")}
	if strings.Contains(roles, "api") {
		os.Args = append(os.Args, "--public-origin=http://"+os.Getenv("GOPAGER_TEST_LISTEN"))
	}
	main()
	os.Exit(0)
}

func TestFullCommandProcessStartupAndSignalShutdown(t *testing.T) {
	t.Parallel()
	url := os.Getenv("GOPAGER_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("GOPAGER_TEST_DATABASE_URL is not set; command/PostgreSQL process evidence not run")
	}
	for _, roles := range []string{"api,worker", "worker"} {
		t.Run(roles, func(t *testing.T) {
			t.Parallel()
			connectionURL := testpostgres.EmptyDatabase(t)
			address := ""
			if roles == "api,worker" {
				listener, err := net.Listen("tcp", "127.0.0.1:0")
				if err != nil {
					t.Fatal(err)
				}
				address = listener.Addr().String()
				if err := listener.Close(); err != nil {
					t.Fatal(err)
				}
			}
			command := exec.Command(os.Args[0], "-test.run=^TestRunProcessHelper$")
			command.Env = append(os.Environ(), "GOPAGER_TEST_PROCESS_HELPER="+roles,
				"GOPAGER_DATABASE_URL="+connectionURL, "GOPAGER_TEST_LISTEN="+address)
			output := startupOutput{ready: make(chan struct{})}
			command.Stdout, command.Stderr = &output, &output
			if err := command.Start(); err != nil {
				t.Fatal(err)
			}
			exited := make(chan error, 1)
			go func() { exited <- command.Wait() }()
			waited := false
			defer func() {
				if !waited {
					_ = command.Process.Kill()
					<-exited
				}
			}()
			deadline := time.Now().Add(platform.DefaultCallTimeout + time.Second)
			ready := false
			client := &http.Client{Timeout: 100 * time.Millisecond}
			for time.Now().Before(deadline) {
				if address != "" {
					response, err := client.Get("http://" + address + "/health/ready")
					if err == nil {
						body, readErr := io.ReadAll(response.Body)
						closeErr := response.Body.Close()
						ready = readErr == nil && closeErr == nil && response.StatusCode == http.StatusOK && string(body) == "{\"ready\":true}\n"
					}
				} else {
					select {
					case <-output.ready:
						ready = true
					default:
					}
				}
				if ready {
					break
				}
				time.Sleep(20 * time.Millisecond)
			}
			if err := command.Process.Signal(os.Interrupt); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-exited:
				waited = true
				if err != nil || !ready {
					t.Fatalf("command ready=%t exit=%v output=%s", ready, err, output.buffer.String())
				}
			case <-time.After(5 * time.Second):
				t.Fatal("Full command did not drain after process signal")
			}
		})
	}
}

type startupOutput struct {
	buffer bytes.Buffer
	ready  chan struct{}
	once   sync.Once
}

// Write records child output and signals completion of the command's startup checks.
func (o *startupOutput) Write(p []byte) (int, error) {
	n, err := o.buffer.Write(p)
	if strings.Contains(o.buffer.String(), "platform started") {
		o.once.Do(func() { close(o.ready) })
	}
	return n, err
}
