package proxy

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"gorm.io/gorm"
)

func TestHostedMediaMaintenanceReportsFailuresAndPreservesRecovery(t *testing.T) {
	for _, phase := range []string{"resume_outstanding", "scan_usage", "scan_retention", "select_queue", "deliver_usage", "expire_terminal_data"} {
		t.Run(phase, func(t *testing.T) {
			database, _, read := newJournalTransactionFixture(t)
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) { writer.WriteHeader(http.StatusBadRequest) }))
			t.Cleanup(upstream.Close)
			core, logs := observer.New(zap.InfoLevel)
			server, service := newHostedMediaAdmissionHTTPServer(t, database, hostedMediaWorkerProvider(upstream.URL), func(service *mediaOperationService) { service.logger = zap.New(core).Sugar() })
			accepted := hostedMediaAdmissionHTTP(t, server, "maintenance-request", "private maintenance prompt", http.StatusAccepted)
			id := accepted["operation_id"].(string)
			service.queued.Delete(id)
			select {
			case <-service.queue:
			default:
			}
			terminal := phase == "deliver_usage" || phase == "expire_terminal_data"
			if terminal {
				service.runOperation("maintenance-fixture", id)
				if current := hostedMediaWorkerStatus(t, server, id); current["state"] != MediaOperationStateFailed {
					t.Fatalf("terminal operation=%v", current)
				}
			}
			if phase == "select_queue" {
				service.dictatorQueue = make(chan string, 1)
			}
			if phase == "deliver_usage" {
				// Supply a pending operational delivery at the database boundary.
				if err := database.database.Where("operation_id = ?", id).Delete(&mediaOperationUsageDeliveryRecord{}).Error; err != nil {
					t.Fatal(err)
				}
			}
			if phase == "expire_terminal_data" {
				now := service.store.now().Add(2 * time.Hour)
				service.store.now = func() time.Time { return now }
			}
			var usageBefore int64
			if err := database.database.Model(&managedUsageEventRecord{}).Count(&usageBefore).Error; err != nil {
				t.Fatal(err)
			}
			var fail atomic.Bool
			fail.Store(true)
			trigger := ""
			if terminal {
				operation := "BEFORE INSERT ON media_operation_usage_delivery_records"
				if phase == "expire_terminal_data" {
					operation = "BEFORE DELETE ON media_operation_records"
				}
				trigger = "reject_maintenance"
				if err := database.database.Exec("CREATE TRIGGER " + trigger + " " + operation + " BEGIN SELECT RAISE(ABORT, 'controlled maintenance failure'); END").Error; err != nil {
					t.Fatal(err)
				}
			} else {
				if err := database.database.Callback().Query().Before("gorm:query").Register("maintenance_read_failure", func(transaction *gorm.DB) {
					if fail.Load() && transaction.Statement.Table == "media_operation_records" {
						transaction.AddError(errors.New("controlled maintenance failure"))
					}
				}); err != nil {
					t.Fatal(err)
				}
			}
			run := func() {
				switch phase {
				case "resume_outstanding":
					service.resumeOutstanding()
				case "scan_usage", "deliver_usage":
					service.deliverPendingUsage()
				case "scan_retention", "expire_terminal_data":
					service.expireTerminalData()
				case "select_queue":
					service.enqueue(id)
				}
			}
			run()
			fail.Store(false)
			if trigger != "" {
				if err := database.database.Exec("DROP TRIGGER " + trigger).Error; err != nil {
					t.Fatal(err)
				}
			}
			reports := logs.FilterMessage("media operation maintenance failed").All()
			if len(reports) != 1 {
				t.Fatalf("maintenance reports=%v", reports)
			}
			fields := reports[0].ContextMap()
			if fields["phase"] != phase || !strings.Contains(fields["error"].(string), "controlled maintenance failure") || strings.Contains(fields["error"].(string), "private maintenance") {
				t.Fatalf("maintenance report=%v", fields)
			}
			if (terminal || phase == "select_queue") && fields["operation_id"] != id {
				t.Fatalf("missing operation identity: %v", fields)
			}
			state := MediaOperationStateQueued
			if terminal {
				state = MediaOperationStateFailed
			}
			if current := hostedMediaWorkerStatus(t, server, id); current["state"] != state {
				t.Fatalf("failed maintenance changed operation=%v", current)
			}
			if len(read("")["requests"].([]any)) != 1 {
				t.Fatal("maintenance changed journal identity")
			}
			var usageAfter, tombstones int64
			if err := database.database.Model(&managedUsageEventRecord{}).Count(&usageAfter).Error; err != nil {
				t.Fatal(err)
			}
			if err := database.database.Model(&mediaOperationTombstoneRecord{}).Count(&tombstones).Error; err != nil {
				t.Fatal(err)
			}
			if usageBefore != usageAfter || tombstones != 0 {
				t.Fatal("failed maintenance partially committed")
			}
			run()
			run()
			if logs.FilterMessage("media operation maintenance failed").Len() != 1 {
				t.Fatal("successful maintenance reported failure")
			}
			switch phase {
			case "resume_outstanding", "select_queue":
				if len(service.queue) != 1 {
					t.Fatalf("recovery queue=%d", len(service.queue))
				}
			case "deliver_usage":
				var count int64
				if err := database.database.Model(&mediaOperationUsageDeliveryRecord{}).Where("operation_id = ?", id).Count(&count).Error; err != nil || count != 1 {
					t.Fatalf("deliveries=%d error=%v", count, err)
				}
			case "expire_terminal_data":
				hostedMediaAdmissionHTTP(t, server, "maintenance-request", "private maintenance prompt", http.StatusGone)
				if len(read("")["requests"].([]any)) != 1 {
					t.Fatal("retention removed financial evidence")
				}
			}
		})
	}
}
