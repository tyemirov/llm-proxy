package proxy

import (
	"path/filepath"
	"runtime"
	"sync"

	"github.com/tyemirov/llm-proxy/internal/openapitest"
)

var internalCanonicalOpenAPIContract = sync.OnceValues(func() (*openapitest.Contract, error) {
	_, sourceFile, _, _ := runtime.Caller(0)
	return openapitest.Load(filepath.Join(filepath.Dir(sourceFile), "..", "..", openapitest.CanonicalDocumentPath))
})
