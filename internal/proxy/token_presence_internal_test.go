package proxy

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestTokenMeasurementEvidenceMigrationRollback(t *testing.T) {
	database, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "migration.sqlite")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	connection, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	if err = database.Exec("CREATE TABLE managed_usage_event_records (id INTEGER PRIMARY KEY, request_tokens INTEGER, response_tokens INTEGER, total_tokens INTEGER)").Error; err != nil {
		t.Fatal(err)
	}
	if err = database.Exec("INSERT INTO managed_usage_event_records VALUES (1, 10, 3, 13)").Error; err != nil {
		t.Fatal(err)
	}
	interrupted := errors.New("test startup interrupted after evidence migration")
	err = database.Transaction(func(transaction *gorm.DB) error {
		if err := migrateManagedTokenMeasurementEvidence(transaction); err != nil {
			return err
		}
		return interrupted
	})
	if !errors.Is(err, interrupted) {
		t.Fatal(err)
	}
	if database.Migrator().HasColumn(&managedUsageEventRecord{}, "MeasurementEvidence") {
		t.Fatal("failed startup left a partial schema migration")
	}
	if err = database.Transaction(migrateManagedTokenMeasurementEvidence); err != nil {
		t.Fatal(err)
	}
	for _, state := range []int{3, 12, 48, 64} {
		if err = database.Exec("UPDATE managed_usage_event_records SET measurement_evidence = ?", state).Error; err != nil {
			t.Fatal(err)
		}
		if err = database.Transaction(migrateManagedTokenMeasurementEvidence); err == nil {
			t.Fatalf("invalid evidence %d accepted", state)
		}
	}
	if err = database.Exec("UPDATE managed_usage_event_records SET measurement_evidence = NULL").Error; err != nil {
		t.Fatal(err)
	}
	if err = database.Transaction(migrateManagedTokenMeasurementEvidence); err != nil {
		t.Fatal(err)
	}
	var record managedUsageEventRecord
	if err = database.First(&record).Error; err != nil {
		t.Fatal(err)
	}
	if record.RequestTokens != 10 || record.ResponseTokens != 3 || record.TotalTokens != 13 || record.MeasurementEvidence != nil {
		t.Fatalf("historical values changed: %+v", record)
	}
}

func TestTokenPresenceNativeQuantities(t *testing.T) {
	for _, fixture := range []struct {
		name, usage string
		counts      [3]int
		states      [3]tokenMeasurementState
	}{
		{"Vertex absent", "", [3]int{}, [3]tokenMeasurementState{}},
		{"Vertex input only", `,"usageMetadata":{"promptTokenCount":10}`, [3]int{10, 0, 0}, [3]tokenMeasurementState{tokenMeasurementComplete, tokenMeasurementUnknown, tokenMeasurementUnknown}},
		{"Vertex thoughts only", `,"usageMetadata":{"thoughtsTokenCount":5}`, [3]int{0, 5, 0}, [3]tokenMeasurementState{tokenMeasurementUnknown, tokenMeasurementPartial, tokenMeasurementUnknown}},
		{"Vertex input and thoughts", `,"usageMetadata":{"promptTokenCount":10,"thoughtsTokenCount":5}`, [3]int{10, 5, 0}, [3]tokenMeasurementState{tokenMeasurementComplete, tokenMeasurementPartial, tokenMeasurementUnknown}},
		{"Vertex zero", `,"usageMetadata":{"promptTokenCount":0,"candidatesTokenCount":0,"totalTokenCount":0}`, [3]int{}, [3]tokenMeasurementState{tokenMeasurementComplete, tokenMeasurementComplete, tokenMeasurementComplete}},
		{"Vertex output and thoughts", `,"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":2,"thoughtsTokenCount":5}`, [3]int{10, 7, 17}, [3]tokenMeasurementState{tokenMeasurementComplete, tokenMeasurementComplete, tokenMeasurementComplete}},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			result, err := parseVertexResponse([]byte(`{"candidates":[{"content":{"parts":[{"text":"answer"}]},"finishReason":"STOP"}]` + fixture.usage + `}`))
			if err != nil {
				t.Fatal(err)
			}
			counts := [3]int{}
			if result.usage != nil {
				counts = [3]int{result.usage.RequestTokens, result.usage.ResponseTokens, result.usage.TotalTokens}
			}
			if counts != fixture.counts {
				t.Fatalf("counts=%v want=%v", counts, fixture.counts)
			}
			evidence := measurementEvidence(result.usage)
			for i, shift := range []uint{tokenRequestShift, tokenResponseShift, tokenTotalShift} {
				if evidence.state(shift) != fixture.states[i] {
					t.Fatalf("quantity %d state=%v want=%v", i, evidence.state(shift), fixture.states[i])
				}
			}
		})
	}
	for _, fixture := range []struct {
		input, output, total *int
		states               [3]tokenMeasurementState
	}{
		{nil, nil, nil, [3]tokenMeasurementState{}},
		{intPointer(0), intPointer(0), intPointer(0), [3]tokenMeasurementState{tokenMeasurementComplete, tokenMeasurementComplete, tokenMeasurementComplete}},
		{intPointer(10), nil, nil, [3]tokenMeasurementState{tokenMeasurementComplete, tokenMeasurementUnknown, tokenMeasurementUnknown}},
	} {
		usage, err := parseChatCompletionTokenUsage(&upstreamTokenUsage{PromptTokens: fixture.input, CompletionTokens: fixture.output, TotalTokens: fixture.total})
		if err != nil {
			t.Fatal(err)
		}
		for i, shift := range []uint{tokenRequestShift, tokenResponseShift, tokenTotalShift} {
			if measurementEvidence(usage).state(shift) != fixture.states[i] {
				t.Fatalf("chat quantity %d evidence=%v", i, measurementEvidence(usage))
			}
		}
	}
}

func TestTokenPresenceHostedReceiptReplay(t *testing.T) {
	for _, fixture := range []struct {
		name                 string
		evidence             *tokenMeasurementEvidence
		input, output, total int
		measured             bool
		invalid              bool
	}{
		{"complete", evidencePointer(tokenMeasurementCompleteEvidence), 10, 3, 13, true, false},
		{"partial", evidencePointer(2), 10, 0, 0, false, false},
		{"historical", nil, 10, 3, 13, false, false},
		{"invalid", evidencePointer(3), 10, 3, 13, false, true},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			database, _, _ := newJournalTransactionFixture(t)
			var calls atomic.Int64
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				io.WriteString(w, `{"id":"receipt","status":"completed","output_text":"saved answer","usage":{"input_tokens":10,"output_tokens":3,"total_tokens":13}}`)
			}))
			t.Cleanup(upstream.Close)
			root := t.TempDir()
			var responses *structuredRequestStore
			server := newHostedIdentityHTTPServer(t, database, upstream.URL, root, func(d *hostedTextRequestDependencies) { responses = d.responses })
			hostedIdentityHTTP(t, server, "evidence", "text", http.StatusOK)
			var accepted managedJournalRequestRecord
			if err := database.database.First(&accepted).Error; err != nil {
				t.Fatal(err)
			}
			record, err := responses.lookupHosted(accepted)
			if err != nil {
				t.Fatal(err)
			}
			stored, err := decodeHostedStoredCompletion(record.Result)
			if err != nil {
				t.Fatal(err)
			}
			if measurementEvidence(stored.Usage) != tokenMeasurementCompleteEvidence {
				t.Fatalf("fresh receipt lost evidence: %+v", stored.Usage)
			}
			stored.Usage = &tokenUsage{RequestTokens: fixture.input, ResponseTokens: fixture.output, TotalTokens: fixture.total, MeasurementEvidence: fixture.evidence}
			encoded, err := json.Marshal(stored)
			if err != nil {
				t.Fatal(err)
			}
			record.Result = encoded
			path, err := responses.recordPath(sha256Hex(accepted.TenantID), accepted.KeyDigest, false)
			if err != nil {
				t.Fatal(err)
			}
			if err := responses.publish(path, record); err != nil {
				t.Fatal(err)
			}
			server.Close()
			server = newHostedIdentityHTTPServer(t, openJournalTransactionInstance(t, database), upstream.URL, root)
			for range 2 {
				request, err := http.NewRequest(http.MethodPost, server.URL+responsesPath, strings.NewReader(`{"model":"openai/gpt-4.1","input":"text"}`))
				if err != nil {
					t.Fatal(err)
				}
				request.Header.Set("Content-Type", "application/json")
				request.Header.Set("Idempotency-Key", "evidence")
				response, err := server.Client().Do(request)
				if err != nil {
					t.Fatal(err)
				}
				var body map[string]any
				json.NewDecoder(response.Body).Decode(&body)
				response.Body.Close()
				want := http.StatusOK
				if fixture.invalid {
					want = http.StatusBadGateway
				}
				if response.StatusCode != want {
					t.Fatalf("replay status=%d want=%d body=%v", response.StatusCode, want, body)
				}
				if !fixture.invalid && (body["usage"] != nil) != fixture.measured {
					t.Fatalf("replay invented evidence: %v", body)
				}
			}
			status := http.StatusOK
			if fixture.invalid {
				status = http.StatusInternalServerError
			}
			publicSaved := hostedIdentityStatusHTTP(t, server, "evidence", status)
			if strings.Contains(publicSaved, "measurement_evidence") {
				t.Fatalf("private evidence leaked: %s", publicSaved)
			}
			if !fixture.invalid {
				var body map[string]any
				if err := json.Unmarshal([]byte(publicSaved), &body); err != nil {
					t.Fatal(err)
				}
				if (body["usage"] != nil) != fixture.measured {
					t.Fatalf("saved status invented measurements: %v", body)
				}
			}
			if calls.Load() != 1 {
				t.Fatalf("replay dispatched %d times", calls.Load())
			}
			if !fixture.invalid {
				decoded, err := decodeHostedStoredCompletion(encoded)
				if err != nil {
					t.Fatal(err)
				}
				if decoded.Usage.RequestTokens != fixture.input || decoded.Usage.ResponseTokens != fixture.output || decoded.Usage.TotalTokens != fixture.total || measurementEvidence(decoded.Usage) != measurementEvidence(stored.Usage) || (decoded.Usage.MeasurementEvidence == nil) != (fixture.evidence == nil) {
					t.Fatalf("stored evidence changed: %+v", decoded.Usage)
				}
			}
		})
	}
}

func evidencePointer(value tokenMeasurementEvidence) *tokenMeasurementEvidence { return &value }

func TestTokenPresenceStartupMigrationFailures(t *testing.T) {
	for _, failure := range []string{"add column", "read evidence", "invalid evidence"} {
		t.Run(failure, func(t *testing.T) {
			cipher, providers := internalManagedProviderKeyCipher(), internalManagementProviderRegistry()
			database, err := newGORMManagedTenantDatabase(ManagementConfiguration{DatabasePath: filepath.Join(t.TempDir(), "startup.sqlite")}, cipher, providers)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := database.close(); err != nil {
					t.Error(err)
				}
			})
			evidence := tokenMeasurementEvidence(0)
			if failure == "invalid evidence" {
				evidence = 3
			}
			if err := database.database.Create(&managedUserRecord{UserID: "historical-owner"}).Error; err != nil {
				t.Fatal(err)
			}
			if err := database.database.Create(&managedTenantRecord{TenantID: "historical", OwnerUserID: "historical-owner", Name: "Historical", NameKey: "historical"}).Error; err != nil {
				t.Fatal(err)
			}
			record := managedUsageEventRecord{TenantID: "historical", StatusCode: 200, OutcomeCode: managedUsageOutcomeSuccess, Disposition: managedUsageDispositionSucceeded, RequestTokens: 10, ResponseTokens: 3, TotalTokens: 13, MeasurementEvidence: &evidence}
			if err := database.database.Create(&record).Error; err != nil {
				t.Fatal(err)
			}
			if failure != "invalid evidence" {
				if err := database.database.Exec("ALTER TABLE managed_usage_event_records DROP COLUMN measurement_evidence").Error; err != nil {
					t.Fatal(err)
				}
			}
			if failure == "add column" {
				if err := database.database.Exec("PRAGMA query_only = ON").Error; err != nil {
					t.Fatal(err)
				}
			}
			if failure == "read evidence" {
				if err := database.database.Callback().Query().Before("gorm:query").Register("token_evidence_read_failure", func(query *gorm.DB) {
					if query.Statement.Table == managedUsageEventTable && len(query.Statement.Selects) == 1 && query.Statement.Selects[0] == "DISTINCT measurement_evidence" {
						query.AddError(errInternalTestDatabase)
					}
				}); err != nil {
					t.Fatal(err)
				}
			}
			err = initializeManagedTenantSchema(database.database, cipher, providers)
			if !errors.Is(err, errManagedTenantSchemaMigration) {
				t.Fatalf("startup accepted %s: %v", failure, err)
			}
			if failure != "invalid evidence" && database.database.Migrator().HasColumn(&managedUsageEventRecord{}, "MeasurementEvidence") {
				t.Fatal("failed startup retained evidence schema")
			}
			if failure == "add column" {
				if err := database.database.Exec("PRAGMA query_only = OFF").Error; err != nil {
					t.Fatal(err)
				}
			}
			if failure == "read evidence" {
				if err := database.database.Callback().Query().Remove("token_evidence_read_failure"); err != nil {
					t.Fatal(err)
				}
			}
			if failure == "invalid evidence" {
				if err := database.database.Exec("UPDATE managed_usage_event_records SET measurement_evidence = NULL").Error; err != nil {
					t.Fatal(err)
				}
			}
			if err := initializeManagedTenantSchema(database.database, cipher, providers); err != nil {
				t.Fatal(err)
			}
			var preserved managedUsageEventRecord
			if err := database.database.First(&preserved, record.ID).Error; err != nil {
				t.Fatal(err)
			}
			if preserved.RequestTokens != 10 || preserved.ResponseTokens != 3 || preserved.TotalTokens != 13 || preserved.MeasurementEvidence != nil {
				t.Fatalf("startup recovery changed historical counts: %+v", preserved)
			}
		})
	}
}
