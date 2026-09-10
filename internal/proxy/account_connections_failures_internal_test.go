package proxy

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Inject storage failures behind the real HTTP handler and transaction. Every
// rejected mutation must leave both the inventory and tenant profile unchanged.
func TestAccountConnectionStorageFailuresAreAtomic(t *testing.T) {
	const connectionTable = "managed_account_connection_records"
	const fieldTable = "managed_connection_field_records"
	const assignmentTable = "managed_tenant_connection_records"
	for _, scenario := range []struct{ action, operation, table string }{
		{"create", "query", managedUserTable},
		{"create", "update", managedUserTable},
		{"create", "create", connectionTable},
		{"create", "create", fieldTable},
		{"create", "query", "managed_connection_creation_records"},
		{"create", "create", "managed_connection_creation_records"},
		{"edit", "query", connectionTable},
		{"edit", "query", fieldTable},
		{"edit", "update", connectionTable},
		{"edit", "delete", fieldTable},
		{"edit", "create", fieldTable},
		{"assign", "query", managedTenantTable},
		{"assign", "query", connectionTable},
		{"assign", "query", assignmentTable},
		{"assign", "create", assignmentTable},
		{"assign", "create", managedProviderProfileTable},
		{"assign", "update", connectionTable},
		{"detach", "query", managedTenantTable},
		{"detach", "query", assignmentTable},
		{"detach", "delete", assignmentTable},
		{"detach", "update", connectionTable},
		{"detach", "update", managedTenantTable},
		{"delete", "query", connectionTable},
		{"delete", "query", assignmentTable},
		{"delete", "delete", fieldTable},
		{"delete", "delete", connectionTable},
		{"prompt", "query", managedTenantTable},
		{"prompt", "query", assignmentTable},
		{"prompt", "update", managedProviderProfileTable},
		{"delete-tenant", "update", connectionTable},
		{"delete-tenant", "delete", assignmentTable},
	} {
		t.Run(scenario.action+"/"+scenario.operation+"/"+scenario.table, func(t *testing.T) {
			_, database, server := newAccountConnectionHTTPFixture(t)
			exchange := func(method, path, body string, status int) map[string]any {
				t.Helper()
				return accountConnectionHTTPExchange(t, server, method, path, body, status)
			}
			connection := exchange(http.MethodPost, "/connections", `{"name":"Production","provider":"openai","fields":{"api_key":"sk-original"}}`, http.StatusCreated)
			connectionID := connection["id"].(string)
			assignmentPath := "/tenants/managed-first/connections/openai"
			if scenario.action == "detach" || scenario.action == "prompt" || scenario.action == "delete-tenant" {
				exchange(http.MethodPut, assignmentPath, fmt.Sprintf(`{"connection_id":%q}`, connectionID), http.StatusOK)
				exchange(http.MethodPut, "/tenants/managed-first/defaults", managementDefaultsBody(ProviderNameOpenAI, ModelNameGPT41), http.StatusOK)
			}
			beforeConnections := exchange(http.MethodGet, "/connections", "", http.StatusOK)
			beforeTenant := exchange(http.MethodGet, "/tenants/managed-first", "", http.StatusOK)
			registerManagedGORMError(t, database.database, "connection-failure", scenario.operation, scenario.table, errInternalTestDatabase)
			method, path, body := http.MethodPost, "/connections", `{"name":"New connection","provider":"openai","fields":{"api_key":"sk-new"}}`
			switch scenario.action {
			case "edit":
				method, path, body = http.MethodPut, "/connections/"+connectionID, `{"name":"Edited","provider":"openai","version":1,"fields":{"api_key":"sk-new"}}`
			case "assign":
				method, path, body = http.MethodPut, assignmentPath, fmt.Sprintf(`{"connection_id":%q}`, connectionID)
			case "detach":
				method, path, body = http.MethodDelete, assignmentPath+"?clear_defaults=true", ""
			case "delete":
				method, path, body = http.MethodDelete, "/connections/"+connectionID, ""
			case "prompt":
				method, path, body = http.MethodPut, "/tenants/managed-first/provider-profiles/openai", `{"text_model":"gpt-4.1","system_prompt":"Changed"}`
			case "delete-tenant":
				method, path, body = http.MethodDelete, "/tenants/managed-first", ""
			}
			exchange(method, path, body, http.StatusInternalServerError)
			removeAccountConnectionFailure(t, database.database, scenario.operation)
			afterConnections := exchange(http.MethodGet, "/connections", "", http.StatusOK)
			afterTenant := exchange(http.MethodGet, "/tenants/managed-first", "", http.StatusOK)
			if !reflect.DeepEqual(beforeConnections, afterConnections) || !reflect.DeepEqual(beforeTenant, afterTenant) {
				t.Fatal("failed mutation changed the saved connection or tenant")
			}
		})
	}
}

func newAccountConnectionHTTPFixture(t *testing.T, middleware ...gin.HandlerFunc) (*managementService, *gormManagedTenantDatabase, *httptest.Server) {
	t.Helper()
	now := time.Date(2026, 9, 10, 16, 0, 0, 0, time.UTC)
	database := newCanonicalGORMFixture(t, now)
	service := newInternalManagementService(t, newFakeManagedTenantDatabase(), internalManagementProviderRegistry())
	service.store.database = database
	service.store.now = func() time.Time { return now }
	principal := managementPrincipal{userID: "owner", userEmail: "owner@example.com"}
	router := gin.New()
	router.Use(service.corsMiddleware(), func(ctx *gin.Context) { ctx.Set(contextKeyManagementPrincipal, principal) })
	router.Use(middleware...)
	group := router.Group(managementAPIPath)
	group.GET(managementConnectionsPath, service.listConnectionsHandler())
	group.POST(managementConnectionsPath, service.saveConnectionHandler(true))
	group.GET(managementConnectionPath, service.getConnectionHandler())
	group.PUT(managementConnectionPath, service.saveConnectionHandler(false))
	group.DELETE(managementConnectionPath, service.deleteConnectionHandler())
	tenants := group.Group(managementTenantPath)
	tenants.GET("", service.tenantProfileHandler())
	tenants.DELETE("", service.deleteTenantHandler())
	tenants.PUT(managementDefaultsPath, service.updateDefaultsHandler())
	tenants.PUT(managementTenantConnectionPath, service.assignConnectionHandler())
	tenants.DELETE(managementTenantConnectionPath, service.detachConnectionHandler())
	tenants.PUT("/provider-profiles/:provider", service.saveTenantProviderProfileHandler())
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	return service, database, server
}

func accountConnectionHTTPExchange(t *testing.T, server *httptest.Server, method, path, body string, status int) map[string]any {
	t.Helper()
	request, err := http.NewRequest(method, server.URL+managementAPIPath+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	if method == http.MethodPost && path == managementConnectionsPath {
		request.Header.Set("Idempotency-Key", rand.Text())
	}
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != status {
		t.Fatalf("%s %s status=%d want=%d body=%s", method, path, response.StatusCode, status, payload)
	}
	if strings.Contains(string(payload), "sk-original") || strings.Contains(string(payload), "sk-new") || strings.Contains(string(payload), errInternalTestDatabase.Error()) {
		t.Fatal("management response exposed credentials or storage details")
	}
	if response.Header.Get("Cache-Control") != "no-store" {
		t.Fatal("management response omitted no-store")
	}
	var result map[string]any
	if len(payload) > 0 && json.Valid(payload) {
		if err := json.Unmarshal(payload, &result); err != nil {
			t.Fatal(err)
		}
	}
	return result
}

func removeAccountConnectionFailure(t *testing.T, database *gorm.DB, operation string) {
	t.Helper()
	var err error
	switch operation {
	case "query":
		err = database.Callback().Query().Remove("connection-failure")
	case "create":
		err = database.Callback().Create().Remove("connection-failure")
	case "update":
		err = database.Callback().Update().Remove("connection-failure")
	case "delete":
		err = database.Callback().Delete().Remove("connection-failure")
	}
	if err != nil {
		t.Fatal(err)
	}
}

func TestAccountConnectionTenantDeletionUsesMutationClock(t *testing.T) {
	service, _, server := newAccountConnectionHTTPFixture(t)
	connection := accountConnectionHTTPExchange(t, server, http.MethodPost, "/connections", `{"name":"Production","provider":"openai","fields":{"api_key":"sk-original"}}`, http.StatusCreated)
	connectionID := connection["id"].(string)
	accountConnectionHTTPExchange(t, server, http.MethodPut, "/tenants/managed-first/connections/openai", fmt.Sprintf(`{"connection_id":%q}`, connectionID), http.StatusOK)
	deletedAt := time.Date(2026, 9, 10, 18, 0, 0, 0, time.UTC)
	service.store.now = func() time.Time { return deletedAt }
	accountConnectionHTTPExchange(t, server, http.MethodDelete, "/tenants/managed-first", "", http.StatusNoContent)
	remaining := accountConnectionHTTPExchange(t, server, http.MethodGet, "/connections/"+connectionID, "", http.StatusOK)
	if remaining["updated_at"] != deletedAt.Format(time.RFC3339) || remaining["version"] != float64(3) || len(remaining["tenant_ids"].([]any)) != 0 {
		t.Fatalf("tenant deletion did not advance the connection with the mutation clock: %+v", remaining)
	}
}

func TestAccountConnectionCreationRetriesReturnOneResource(t *testing.T) {
	service, database, server := newAccountConnectionHTTPFixture(t)
	body := `{"name":"Retry-safe creation","provider":"openai","fields":{"api_key":"sk-original"}}`
	create := func(payload string, status int) map[string]any {
		t.Helper()
		request, err := http.NewRequest(http.MethodPost, server.URL+managementAPIPath+managementConnectionsPath, strings.NewReader(payload))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Idempotency-Key", "connection-creation-retry")
		response, err := server.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if response.StatusCode != status {
			t.Fatalf("create status=%d want=%d", response.StatusCode, status)
		}
		var resource map[string]any
		if err := json.NewDecoder(response.Body).Decode(&resource); err != nil {
			t.Fatal(err)
		}
		return resource
	}
	first := create(body, http.StatusCreated)
	repeated := create(body, http.StatusCreated)
	if !reflect.DeepEqual(first, repeated) {
		t.Fatal("repeated creation returned another resource or changed its representation")
	}
	// A fresh adapter must use the durable receipt without consuming entropy.
	service.store.database = &gormManagedTenantDatabase{database: database.database}
	service.store.randomReader = strings.NewReader("")
	if !reflect.DeepEqual(first, create(body, http.StatusCreated)) {
		t.Fatal("a fresh adapter did not replay the durable receipt")
	}
	create(strings.Replace(body, "Retry-safe creation", "Another request", 1), http.StatusConflict)
	inventory := accountConnectionHTTPExchange(t, server, http.MethodGet, managementConnectionsPath, "", http.StatusOK)
	if len(inventory["connections"].([]any)) != 1 {
		t.Fatal("repeated creation added another connection")
	}
	accountConnectionHTTPExchange(t, server, http.MethodDelete, "/connections/"+first["id"].(string), "", http.StatusNoContent)
	if !reflect.DeepEqual(first, create(body, http.StatusCreated)) {
		t.Fatal("deleted connection receipt changed")
	}
	inventory = accountConnectionHTTPExchange(t, server, http.MethodGet, managementConnectionsPath, "", http.StatusOK)
	if len(inventory["connections"].([]any)) != 0 {
		t.Fatal("retry recreated a deleted connection")
	}
}

func TestAccountConnectionCreationRequiresOneValidRetryKey(t *testing.T) {
	_, _, server := newAccountConnectionHTTPFixture(t)
	for _, keys := range [][]string{nil, {""}, {"space invalid"}, {"first", "second"}, {strings.Repeat("a", 129)}} {
		request, err := http.NewRequest(http.MethodPost, server.URL+managementAPIPath+managementConnectionsPath, strings.NewReader(`{"name":"Invalid header","provider":"openai","fields":{"api_key":"sk-original"}}`))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/json")
		for _, key := range keys {
			request.Header.Add(managementIdempotencyHeader, key)
		}
		response, err := server.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusBadRequest {
			t.Fatalf("keys=%q status=%d", keys, response.StatusCode)
		}
	}
	inventory := accountConnectionHTTPExchange(t, server, http.MethodGet, managementConnectionsPath, "", http.StatusOK)
	if len(inventory["connections"].([]any)) != 0 {
		t.Fatal("invalid retry header created a connection")
	}
}

type blockedConnectionVerification struct {
	started chan struct{}
	release chan struct{}
}

func (verifier blockedConnectionVerification) verify(ctx context.Context, _ providerDefinition, _ textModelDefinition, _ string) error {
	verifier.started <- struct{}{}
	select {
	case <-verifier.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestAccountConnectionConcurrentCreationReceipts(t *testing.T) {
	for _, changed := range []bool{false, true} {
		t.Run(fmt.Sprint(changed), func(t *testing.T) {
			service, _, server := newAccountConnectionHTTPFixture(t)
			verifier := blockedConnectionVerification{make(chan struct{}, 2), make(chan struct{})}
			service.keyVerifier = verifier
			var once sync.Once
			release := func() { once.Do(func() { close(verifier.release) }) }
			t.Cleanup(release)
			results := make(chan *http.Response, 2)
			failures := make(chan error, 2)
			for index := 0; index < 2; index++ {
				name := "Concurrent"
				if changed && index == 1 {
					name = "Conflicting"
				}
				request, err := http.NewRequest(http.MethodPost, server.URL+managementAPIPath+managementConnectionsPath, strings.NewReader(fmt.Sprintf(`{"name":%q,"provider":"openai","fields":{"api_key":"sk-original"}}`, name)))
				if err != nil {
					t.Fatal(err)
				}
				request.Header.Set("Content-Type", "application/json")
				request.Header.Set(managementIdempotencyHeader, "concurrent-creation")
				go func() {
					response, err := server.Client().Do(request)
					if err != nil {
						failures <- err
						return
					}
					results <- response
				}()
			}
			for index := 0; index < 2; index++ {
				select {
				case <-verifier.started:
				case err := <-failures:
					t.Fatal(err)
				case <-time.After(5 * time.Second):
					t.Fatal("concurrent verification did not begin")
				}
			}
			release()
			statuses := map[int]int{}
			for index := 0; index < 2; index++ {
				select {
				case response := <-results:
					statuses[response.StatusCode]++
					response.Body.Close()
				case err := <-failures:
					t.Fatal(err)
				case <-time.After(5 * time.Second):
					t.Fatal("concurrent creation did not finish")
				}
			}
			expected := map[int]int{http.StatusCreated: 2}
			if changed {
				expected = map[int]int{http.StatusCreated: 1, http.StatusConflict: 1}
			}
			if !reflect.DeepEqual(statuses, expected) {
				t.Fatalf("creation statuses=%v want=%v", statuses, expected)
			}
			inventory := accountConnectionHTTPExchange(t, server, http.MethodGet, managementConnectionsPath, "", http.StatusOK)
			if len(inventory["connections"].([]any)) != 1 {
				t.Fatal("concurrent creation duplicated connection")
			}
		})
	}
}

func TestAccountConnectionCanceledChangesPreserveState(t *testing.T) {
	for _, operation := range []string{"assign", "detach", "delete", "prompt"} {
		t.Run(operation, func(t *testing.T) {
			var cancelRequests atomic.Bool
			_, _, server := newAccountConnectionHTTPFixture(t, func(ctx *gin.Context) {
				if cancelRequests.Load() {
					requestContext, cancel := context.WithCancel(ctx.Request.Context())
					cancel()
					ctx.Request = ctx.Request.WithContext(requestContext)
				}
			})
			connection := accountConnectionHTTPExchange(t, server, http.MethodPost, managementConnectionsPath, `{"name":"Cancellation","provider":"openai","fields":{"api_key":"sk-original"}}`, http.StatusCreated)
			path := "/tenants/managed-first/connections/openai"
			body := fmt.Sprintf(`{"connection_id":%q}`, connection["id"])
			if operation == "detach" || operation == "prompt" {
				accountConnectionHTTPExchange(t, server, http.MethodPut, path, body, http.StatusOK)
			}
			before := accountConnectionHTTPExchange(t, server, http.MethodGet, managementConnectionsPath, "", http.StatusOK)
			method := http.MethodPut
			switch operation {
			case "detach":
				method = http.MethodDelete
				body = ""
			case "delete":
				method = http.MethodDelete
				path = managementConnectionsPath + "/" + connection["id"].(string)
				body = ""
			case "prompt":
				path = "/tenants/managed-first/provider-profiles/openai"
				body = `{"text_model":"gpt-4.1","system_prompt":"Changed"}`
			}
			cancelRequests.Store(true)
			accountConnectionHTTPExchange(t, server, method, path, body, http.StatusInternalServerError)
			cancelRequests.Store(false)
			after := accountConnectionHTTPExchange(t, server, http.MethodGet, managementConnectionsPath, "", http.StatusOK)
			if !reflect.DeepEqual(before, after) {
				t.Fatal("canceled mutation changed connections")
			}
		})
	}
}

func TestAccountConnectionInvalidTenantAndProviderInputs(t *testing.T) {
	_, _, server := newAccountConnectionHTTPFixture(t)
	for _, method := range []string{http.MethodPut, http.MethodDelete} {
		accountConnectionHTTPExchange(t, server, method, "/tenants/%20/connections/openai", `{"connection_id":"missing"}`, http.StatusNotFound)
		accountConnectionHTTPExchange(t, server, method, "/tenants/managed-first/connections/unknown", `{"connection_id":"missing"}`, http.StatusBadRequest)
	}
	accountConnectionHTTPExchange(t, server, http.MethodPut, "/tenants/%20/provider-profiles/openai", `{"text_model":"gpt-4.1"}`, http.StatusNotFound)
	for _, scenario := range []struct{ provider, body string }{
		{"unknown", `{"text_model":"gpt-4.1"}`}, {"openai", `{"text_model":"not-a-model"}`}, {"openai", `{"text_model":""}`}, {"openai", `{"unknown":true}`},
	} {
		accountConnectionHTTPExchange(t, server, http.MethodPut, "/tenants/managed-first/provider-profiles/"+scenario.provider, scenario.body, http.StatusBadRequest)
	}
	accountConnectionHTTPExchange(t, server, http.MethodPut, "/tenants/managed-first/provider-profiles/openai", `{"text_model":"gpt-4.1"}`, http.StatusNotFound)
	accountConnectionHTTPExchange(t, server, http.MethodDelete, "/tenants/managed-first/connections/openai?clear_defaults=invalid", "", http.StatusBadRequest)
}

func TestAccountConnectionReadFailuresAndEntropyFailures(t *testing.T) {
	for _, table := range []string{managedUserTable, "managed_account_connection_records", "managed_connection_field_records"} {
		t.Run(table, func(t *testing.T) {
			_, database, server := newAccountConnectionHTTPFixture(t)
			accountConnectionHTTPExchange(t, server, http.MethodPost, managementConnectionsPath, `{"name":"Read failure","provider":"openai","fields":{"api_key":"sk-original"}}`, http.StatusCreated)
			registerManagedGORMError(t, database.database, "connection-failure", "query", table, errInternalTestDatabase)
			accountConnectionHTTPExchange(t, server, http.MethodGet, managementConnectionsPath, "", http.StatusInternalServerError)
			removeAccountConnectionFailure(t, database.database, "query")
		})
	}
	for _, entropy := range []string{"", strings.Repeat("x", 16)} {
		service, _, server := newAccountConnectionHTTPFixture(t)
		service.store.randomReader = strings.NewReader(entropy)
		accountConnectionHTTPExchange(t, server, http.MethodPost, managementConnectionsPath, `{"name":"Entropy failure","provider":"openai","fields":{"api_key":"sk-original"}}`, http.StatusInternalServerError)
		inventory := accountConnectionHTTPExchange(t, server, http.MethodGet, managementConnectionsPath, "", http.StatusOK)
		if len(inventory["connections"].([]any)) != 0 {
			t.Fatal("failed entropy created a connection")
		}
	}
}

type connectionVerificationFunc func(context.Context) error

func (verify connectionVerificationFunc) verify(ctx context.Context, _ providerDefinition, _ textModelDefinition, _ string) error {
	return verify(ctx)
}

type connectionCancellationKey struct{}

func TestAccountConnectionCancellationAfterVerification(t *testing.T) {
	for _, edit := range []bool{false, true} {
		service, _, server := newAccountConnectionHTTPFixture(t, func(ctx *gin.Context) {
			requestContext, cancel := context.WithCancel(ctx.Request.Context())
			defer cancel()
			ctx.Request = ctx.Request.WithContext(context.WithValue(requestContext, connectionCancellationKey{}, cancel))
			ctx.Next()
		})
		created := accountConnectionHTTPExchange(t, server, http.MethodPost, managementConnectionsPath, `{"name":"Existing","provider":"openai","fields":{"api_key":"sk-original"}}`, http.StatusCreated)
		method, path, body := http.MethodPost, managementConnectionsPath, `{"name":"Canceled","provider":"openai","fields":{"api_key":"sk-new"}}`
		if edit {
			method = http.MethodPut
			path += "/" + created["id"].(string)
			body = `{"name":"Canceled","provider":"openai","version":1,"fields":{"api_key":"sk-new"}}`
		}
		before := accountConnectionHTTPExchange(t, server, http.MethodGet, managementConnectionsPath, "", http.StatusOK)
		service.keyVerifier = connectionVerificationFunc(func(ctx context.Context) error {
			ctx.Value(connectionCancellationKey{}).(context.CancelFunc)()
			return nil
		})
		accountConnectionHTTPExchange(t, server, method, path, body, http.StatusInternalServerError)
		after := accountConnectionHTTPExchange(t, server, http.MethodGet, managementConnectionsPath, "", http.StatusOK)
		if !reflect.DeepEqual(before, after) {
			t.Fatal("canceled credential verification changed persisted state")
		}
	}
}

func TestAccountConnectionCreationReceiptRecheckFailureRollsBack(t *testing.T) {
	_, database, server := newAccountConnectionHTTPFixture(t)
	reads := 0
	if err := database.database.Callback().Query().Before("gorm:query").Register("second_receipt_read", func(tx *gorm.DB) {
		if tx.Statement.Table == "managed_connection_creation_records" {
			reads++
			if reads == 2 {
				tx.AddError(errInternalTestDatabase)
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	accountConnectionHTTPExchange(t, server, http.MethodPost, managementConnectionsPath, `{"name":"Receipt failure","provider":"openai","fields":{"api_key":"sk-original"}}`, http.StatusInternalServerError)
	inventory := accountConnectionHTTPExchange(t, server, http.MethodGet, managementConnectionsPath, "", http.StatusOK)
	if len(inventory["connections"].([]any)) != 0 {
		t.Fatal("receipt recheck failure created a connection")
	}
}

func TestAccountConnectionOptionalCredentialsAndInvalidAssignments(t *testing.T) {
	service, _, server := newAccountConnectionHTTPFixture(t)
	definition := service.providers.definitions[providerID(ProviderNameOpenAI)]
	field := definition.fields["api_key"]
	field.Required = false
	definition.fields["api_key"] = field
	body := `{"name":"Optional credential","provider":"openai","fields":{"api_key":""}}`
	created := accountConnectionHTTPExchange(t, server, http.MethodPost, managementConnectionsPath, body, http.StatusCreated)
	accountConnectionHTTPExchange(t, server, http.MethodPut, managementConnectionsPath+"/"+created["id"].(string), `{"name":"Renamed","provider":"openai","version":1,"fields":{"api_key":""}}`, http.StatusOK)
	accountConnectionHTTPExchange(t, server, http.MethodPost, managementConnectionsPath, `{"name":"Invalid version","provider":"openai","version":1,"fields":{"api_key":""}}`, http.StatusBadRequest)
	path := "/tenants/managed-first/connections/openai"
	for _, body := range []string{`{}`, `{`, `{"connection_id":""}`, `{"connection_id":"missing"}`} {
		status := http.StatusBadRequest
		if strings.Contains(body, "missing") {
			status = http.StatusNotFound
		}
		accountConnectionHTTPExchange(t, server, http.MethodPut, path, body, status)
	}
	accountConnectionHTTPExchange(t, server, http.MethodDelete, path, "", http.StatusNoContent)
}

func TestAccountConnectionRetryAfterProfileReadFailure(t *testing.T) {
	for _, action := range []string{"assign", "prompt"} {
		_, database, server := newAccountConnectionHTTPFixture(t)
		created := accountConnectionHTTPExchange(t, server, http.MethodPost, managementConnectionsPath, `{"name":"Read retry","provider":"openai","fields":{"api_key":"sk-original"}}`, http.StatusCreated)
		path := "/tenants/managed-first/connections/openai"
		body := fmt.Sprintf(`{"connection_id":%q}`, created["id"])
		if action == "prompt" {
			accountConnectionHTTPExchange(t, server, http.MethodPut, path, body, http.StatusOK)
			path = "/tenants/managed-first/provider-profiles/openai"
			body = `{"text_model":"gpt-4.1","system_prompt":"Saved prompt"}`
		}
		registerManagedGORMError(t, database.database, "connection-failure", "query", managedProviderProfileTable, errInternalTestDatabase)
		accountConnectionHTTPExchange(t, server, http.MethodPut, path, body, http.StatusInternalServerError)
		removeAccountConnectionFailure(t, database.database, "query")
		accountConnectionHTTPExchange(t, server, http.MethodPut, path, body, http.StatusOK)
		inventory := accountConnectionHTTPExchange(t, server, http.MethodGet, managementConnectionsPath, "", http.StatusOK)
		connection := inventory["connections"].([]any)[0].(map[string]any)
		if len(connection["tenant_ids"].([]any)) != 1 {
			t.Fatal("retry duplicated assignment")
		}
	}
}

func TestAccountConnectionRejectsCorruptedFieldsAndMissingProfiles(t *testing.T) {
	service, database, server := newAccountConnectionHTTPFixture(t)
	connection := accountConnectionHTTPExchange(t, server, http.MethodPost, managementConnectionsPath, `{"name":"Corruption","provider":"openai","fields":{"api_key":"sk-original"}}`, http.StatusCreated)
	accountConnectionHTTPExchange(t, server, http.MethodPut, "/tenants/managed-first/connections/openai", fmt.Sprintf(`{"connection_id":%q}`, connection["id"]), http.StatusOK)
	if err := database.database.Where("tenant_id = ?", "managed-first").Delete(&managedProviderProfileRecord{}).Error; err != nil {
		t.Fatal(err)
	}
	accountConnectionHTTPExchange(t, server, http.MethodPut, "/tenants/managed-first/defaults", managementDefaultsBody(ProviderNameOpenAI, ModelNameGPT41), http.StatusInternalServerError)
	if err := database.database.Model(&managedConnectionFieldRecord{}).Where("connection_id = ?", connection["id"]).Update("value", "corrupted").Error; err != nil {
		t.Fatal(err)
	}
	accountConnectionHTTPExchange(t, server, http.MethodGet, managementConnectionsPath, "", http.StatusInternalServerError)
	// A URL constraint comes from the provider catalog at the boundary.
	definition := service.providers.definitions[providerID(ProviderNameOpenAI)]
	field := definition.fields["api_key"]
	field.Secret = false
	field.Type = CatalogProviderFieldTypeURL
	field.Validation = ProviderCatalogFieldValidation{AllowedSchemes: []string{"https"}}
	definition.fields["api_key"] = field
	accountConnectionHTTPExchange(t, server, http.MethodGet, managementConnectionsPath, "", http.StatusInternalServerError)
	accountConnectionHTTPExchange(t, server, http.MethodPost, managementConnectionsPath, `{"name":"Invalid URL","provider":"openai","fields":{"api_key":"invalid"}}`, http.StatusBadRequest)
}

func TestAccountConnectionDetachRequiresDefaultResolution(t *testing.T) {
	_, _, server := newAccountConnectionHTTPFixture(t)
	connection := accountConnectionHTTPExchange(t, server, http.MethodPost, managementConnectionsPath, `{"name":"Default route","provider":"openai","fields":{"api_key":"sk-original"}}`, http.StatusCreated)
	path := "/tenants/managed-first/connections/openai"
	accountConnectionHTTPExchange(t, server, http.MethodPut, path, fmt.Sprintf(`{"connection_id":%q}`, connection["id"]), http.StatusOK)
	accountConnectionHTTPExchange(t, server, http.MethodPut, "/tenants/managed-first/defaults", managementDefaultsBody(ProviderNameOpenAI, ModelNameGPT41), http.StatusOK)
	before := accountConnectionHTTPExchange(t, server, http.MethodGet, "/tenants/managed-first", "", http.StatusOK)
	accountConnectionHTTPExchange(t, server, http.MethodDelete, path, "", http.StatusConflict)
	after := accountConnectionHTTPExchange(t, server, http.MethodGet, "/tenants/managed-first", "", http.StatusOK)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("unconfirmed detach changed the tenant")
	}
	accountConnectionHTTPExchange(t, server, http.MethodDelete, path+"?clear_defaults=true", "", http.StatusNoContent)
	accountConnectionHTTPExchange(t, server, http.MethodDelete, path, "", http.StatusNoContent)
}
