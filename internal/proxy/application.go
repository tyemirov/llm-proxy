package proxy

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

const hostedFundsReconciliationInterval = time.Second

type proxyApplication struct {
	router   *gin.Engine
	database managedTenantDatabase
	address  string
	now      func() time.Time
}

// Serve runs HTTP and financial reconciliation under one process lifecycle.
func Serve(configuration Configuration, structuredLogger *zap.SugaredLogger) error {
	application, err := buildProxyApplication(configuration, structuredLogger, newManagedTenantStore)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	listener, err := net.Listen("tcp", application.address)
	if err != nil {
		return fmt.Errorf("listen for proxy requests: %w", err)
	}
	return application.serve(ctx, listener)
}

func (application *proxyApplication) serve(ctx context.Context, listener net.Listener) error {
	if err := application.database.reconcileHostedFunds(ctx, application.now().UTC()); err != nil {
		return errors.Join(fmt.Errorf("initialize funds reconciliation: %w", err), listener.Close())
	}
	group, running := errgroup.WithContext(ctx)
	server := &http.Server{Handler: application.router, BaseContext: func(net.Listener) context.Context { return running }}
	group.Go(func() error {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve proxy requests: %w", err)
		}
		return nil
	})
	group.Go(func() error {
		ticker := time.NewTicker(hostedFundsReconciliationInterval)
		defer ticker.Stop()
		return application.reconcileFunds(running, ticker.C)
	})
	group.Go(func() error {
		<-running.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), journalPersistenceTimeout)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			return errors.Join(fmt.Errorf("stop proxy requests: %w", err), server.Close())
		}
		return nil
	})
	return group.Wait()
}

func (application *proxyApplication) reconcileFunds(ctx context.Context, ticks <-chan time.Time) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticks:
			if err := application.database.reconcileHostedFunds(ctx, application.now().UTC()); err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return fmt.Errorf("reconcile hosted funds: %w", err)
			}
		}
	}
}
