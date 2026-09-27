package proxy

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/signal"
	"reflect"
	"sync"
	"syscall"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestHostedPaymentsPublicServeShutdownPreservesFundedAccount(t *testing.T) {
	fixture := newPaymentStartupFixture(t)
	process, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	// Serve owns process signals. Keep this test sequential and retain a second
	// handler until shutdown completes, including if Serve exits unexpectedly.
	for range 2 {
		func() {
			observed := make(chan os.Signal, 1)
			signal.Notify(observed, syscall.SIGTERM)
			defer signal.Stop(observed)
			listener, err := net.Listen("tcp", ":0")
			if err != nil {
				t.Fatal(err)
			}
			configuration := fixture.configuration
			configuration.Port = listener.Addr().(*net.TCPAddr).Port
			if err := listener.Close(); err != nil {
				t.Fatal(err)
			}
			address := fmt.Sprintf("127.0.0.1:%d", configuration.Port)
			exited := make(chan struct{})
			var serveError error
			go func() {
				serveError = Serve(configuration, zap.NewNop().Sugar())
				close(exited)
			}()
			stop := sync.OnceFunc(func() {
				select {
				case <-exited:
				default:
					if err := process.Signal(syscall.SIGTERM); err != nil {
						t.Error(err)
						return
					}
					select {
					case <-observed:
					case <-time.After(5 * time.Second):
						t.Error("termination signal was not delivered")
						return
					}
					select {
					case <-exited:
					case <-time.After(5 * time.Second):
						t.Error("public Serve did not stop after termination")
						return
					}
				}
				if serveError != nil {
					t.Errorf("public Serve failed: %v", serveError)
				}
			})
			defer stop()
			client := &http.Client{Timeout: time.Second}
			deadline := time.NewTimer(5 * time.Second)
			defer deadline.Stop()
			tick := time.NewTicker(20 * time.Millisecond)
			defer tick.Stop()
			for {
				response, err := client.Get("http://" + address + healthPath)
				if err == nil {
					response.Body.Close()
					if response.StatusCode == http.StatusOK {
						break
					}
				}
				select {
				case <-exited:
					t.Fatalf("public Serve exited before readiness: %v", serveError)
				case <-deadline.C:
					t.Fatal("public Serve did not become ready")
				case <-tick.C:
				}
			}
			live := fixture.paymentAuditFixture
			live.server = &httptest.Server{URL: "http://" + address}
			if !reflect.DeepEqual(fixture.before, paymentAdjustmentResources(t, live)) {
				t.Fatal("public startup changed funded resources")
			}
			stop()
			connection, err := net.DialTimeout("tcp", address, time.Second)
			if err == nil {
				connection.Close()
				t.Fatal("public shutdown retained its listener")
			}
			if !reflect.DeepEqual(fixture.before, paymentAdjustmentResources(t, fixture.paymentAuditFixture)) || fixture.processor.creates.Load() != 1 {
				t.Fatal("public shutdown changed funds or repeated payment creation")
			}
		}()
	}
}
