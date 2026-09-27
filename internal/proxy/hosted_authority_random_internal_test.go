package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestHostedAuthorityRandomSourceFailureRetainsAcceptedState(t *testing.T) {
	for _, scenario := range []string{"connection-id", "credential-encryption", "grant-id"} {
		t.Run(scenario, func(t *testing.T) {
			database, _, _ := newJournalTransactionFixture(t)
			service := newInternalManagementService(t, newFakeManagedTenantDatabase(), internalManagementProviderRegistry())
			service.store.database = database
			server, cookie := fundsManagementServiceHTTPFixture(t, service)
			exchange := func(method, path, body, key string, status int) []byte {
				t.Helper()
				request, err := http.NewRequest(method, server.URL+managementAPIPath+path, strings.NewReader(body))
				if err != nil {
					t.Fatal(err)
				}
				request.AddCookie(cookie("operator"))
				request.Header.Set("Origin", service.configuration.PublicOrigin)
				request.Header.Set("Content-Type", "application/json")
				if key != "" {
					request.Header.Set(managementIdempotencyHeader, key)
				}
				response, err := server.Client().Do(request)
				if err != nil {
					t.Fatal(err)
				}
				defer response.Body.Close()
				payload, err := io.ReadAll(response.Body)
				if err != nil || response.StatusCode != status {
					t.Fatalf("%s %s status=%d want=%d body=%s error=%v", method, path, response.StatusCode, status, payload, err)
				}
				if strings.Contains(string(payload), "sk-private") || response.Header.Get("Cache-Control") != "no-store" {
					t.Fatal("authority response exposed or cached private data")
				}
				return payload
			}
			counts := func() []int64 {
				t.Helper()
				var result []int64
				for _, table := range []string{"managed_hosted_creation_records", "managed_platform_connection_records", "managed_platform_credential_records", "managed_hosted_grant_records", "managed_hosted_grant_revision_records"} {
					var count int64
					if err := database.database.Table(table).Count(&count).Error; err != nil {
						t.Fatal(err)
					}
					result = append(result, count)
				}
				return result
			}
			path := managementPlatformConnectionsPath
			body := `{"name":"Entropy test","provider":"openai","fields":{"api_key":"sk-private"}}`
			status := http.StatusInternalServerError
			if scenario == "grant-id" {
				path = managementHostedGrantsPath
				body = fmt.Sprintf(`{"billing_account_id":"billing-journal","tenant_id":"managed-first","platform_connection_id":"platform-journal","catalog_revision":%q,"offerings":[{"model":"gpt-4.1","operations":["text"]}],"reason":"Entropy test"}`, service.providers.catalog.modelCatalog.Revision)
			}
			before := exchange(http.MethodGet, path, "", "", http.StatusOK)
			beforeCounts := counts()
			original := service.store.randomReader
			service.store.randomReader = strings.NewReader("")
			if scenario == "credential-encryption" {
				service.store.randomReader = bytes.NewReader(bytes.Repeat([]byte{7}, hostedResourceIDBytes))
				status = http.StatusServiceUnavailable
			}
			exchange(http.MethodPost, path, body, "random-recovery", status)
			service.store.randomReader = original
			after := exchange(http.MethodGet, path, "", "", http.StatusOK)
			if !bytes.Equal(before, after) || !reflect.DeepEqual(beforeCounts, counts()) {
				t.Fatal("random-source failure committed partial authority")
			}
			accepted := exchange(http.MethodPost, path, body, "random-recovery", http.StatusCreated)
			var resource struct{ ID string }
			if err := json.Unmarshal(accepted, &resource); err != nil || resource.ID == "" {
				t.Fatalf("random-source recovery has no resource: body=%s error=%v", accepted, err)
			}
			recoveredCounts := counts()
			replayed := exchange(http.MethodPost, path, body, "random-recovery", http.StatusCreated)
			if !bytes.Equal(accepted, replayed) || !reflect.DeepEqual(recoveredCounts, counts()) {
				t.Fatal("random-source recovery duplicated authority")
			}
		})
	}
}
