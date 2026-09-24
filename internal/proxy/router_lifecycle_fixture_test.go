package proxy

import (
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func buildRouterForTest(t testing.TB, configuration Configuration, logger *zap.SugaredLogger) (*gin.Engine, error) {
	t.Helper()
	return buildRouterWithStoreForTest(t, configuration, logger, newManagedTenantStore)
}

func buildRouterWithStoreForTest(t testing.TB, configuration Configuration, logger *zap.SugaredLogger, openStore managedTenantStoreOpener) (*gin.Engine, error) {
	t.Helper()
	router, err := buildRouter(configuration, logger, openStore)
	if err != nil {
		return nil, err
	}
	t.Cleanup(router.Close)
	return router.Engine, nil
}

func buildProxyApplicationForTest(t testing.TB, configuration Configuration, logger *zap.SugaredLogger, openStore managedTenantStoreOpener) (*proxyApplication, error) {
	t.Helper()
	application, err := buildProxyApplication(configuration, logger, openStore)
	if err != nil {
		return nil, err
	}
	t.Cleanup(application.close)
	return application, nil
}
