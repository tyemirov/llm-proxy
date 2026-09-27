package proxy

import (
	"context"
	"errors"
	"sync"

	"github.com/tyemirov/llm-proxy/internal/constants"
	"go.uber.org/zap"
)

var errManagedUsageQueueFull = errors.New("managed_usage_queue_full")
var errManagedUsageWriterClosed = errors.New("managed_usage_writer_closed")

type managedUsageWrite struct {
	record           managedUsageEventRecord
	structuredLogger *zap.SugaredLogger
}

type managedUsageWriter struct {
	store     *managedTenantStore
	queue     chan managedUsageWrite
	startOnce sync.Once
	mutex     sync.RWMutex
	closed    bool
	done      chan struct{}
}

func newManagedUsageWriter(store *managedTenantStore, queueSize int) *managedUsageWriter {
	return &managedUsageWriter{
		store: store,
		queue: make(chan managedUsageWrite, queueSize),
		done:  make(chan struct{}),
	}
}

func (writer *managedUsageWriter) submit(requestTenant tenant, event managedUsageEvent, structuredLogger *zap.SugaredLogger) {
	usageRecord, recordError := writer.store.newManagedUsageRecord(requestTenant, event)
	submission := managedUsageWrite{
		record:           usageRecord,
		structuredLogger: structuredLogger,
	}
	if recordError != nil {
		writer.log(logEventUsageRecordFailed, submission, recordError)
		return
	}
	writer.mutex.RLock()
	defer writer.mutex.RUnlock()
	if writer.closed {
		writer.log(logEventUsageRecordDropped, submission, errManagedUsageWriterClosed)
		return
	}
	select {
	case writer.queue <- submission:
		writer.startOnce.Do(func() {
			go writer.run()
		})
	default:
		writer.log(logEventUsageRecordDropped, submission, errManagedUsageQueueFull)
	}
}

func (writer *managedUsageWriter) run() {
	defer close(writer.done)
	for submission := range writer.queue {
		writer.persist(submission)
	}
}

func (writer *managedUsageWriter) close() {
	writer.mutex.Lock()
	if !writer.closed {
		writer.closed = true
		close(writer.queue)
	}
	writer.mutex.Unlock()
	writer.startOnce.Do(func() { close(writer.done) })
	<-writer.done
}

func (writer *managedUsageWriter) persist(submission managedUsageWrite) {
	persistenceContext, cancelPersistence := context.WithTimeout(context.Background(), managedUsagePersistenceTimeout)
	defer cancelPersistence()
	if recordError := writer.store.persistManagedUsageRecord(persistenceContext, submission.record); recordError != nil {
		writer.log(logEventUsageRecordFailed, submission, recordError)
	}
}

func (writer *managedUsageWriter) log(eventName string, submission managedUsageWrite, recordError error) {
	submission.structuredLogger.Warnw(
		eventName,
		logFieldTenantID, submission.record.TenantID,
		logFieldEndpoint, submission.record.Endpoint,
		logFieldProvider, submission.record.ProviderID,
		logFieldModel, submission.record.ModelID,
		logFieldStatus, submission.record.StatusCode,
		constants.LogFieldError, recordError,
	)
}
