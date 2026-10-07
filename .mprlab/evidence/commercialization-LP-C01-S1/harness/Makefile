GO ?= go

.PHONY: benchmark
benchmark:
	GOCACHE=/tmp/llm-proxy-evaluation-go-cache GOPROXY=off GOSUMDB=off $(GO) test -mod=readonly -run '^TestExtractionBenchmark$$' -count=1 -v .
