package proxy

import (
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"reflect"
	"sync/atomic"
	"testing"

	"gorm.io/gorm"
)

const grantTransitionRecoveryPath = "/hosted-access-grants/grant-journal"

func TestHostedGrantTransitionReadFailurePreservesRevisionHistory(t *testing.T) {
	for _, state := range []hostedGrantState{hostedGrantSuspended, hostedGrantActive, hostedGrantRevoked} {
		t.Run(string(state), func(t *testing.T) {
			database, _, _ := newJournalTransactionFixture(t)
			server, cookie := newFundsManagementHTTPFixture(t, database)
			exchange := func(method, path, body string, status int) map[string]any {
				t.Helper()
				user := "operator"
				if path == "/tenants/managed-first/connections" || path == fundsBalanceTestPath {
					user = "owner"
				}
				jar, err := cookiejar.New(nil)
				if err != nil {
					t.Fatal(err)
				}
				origin, err := url.Parse(server.URL)
				if err != nil {
					t.Fatal(err)
				}
				jar.SetCookies(origin, []*http.Cookie{cookie(user)})
				server.Client().Jar = jar
				return accountConnectionHTTPExchange(t, server, method, path, body, status)
			}
			snapshot := func() map[string]any {
				t.Helper()
				resources := map[string]any{}
				for _, path := range []string{grantTransitionRecoveryPath, grantTransitionRecoveryPath + "/revisions", "/tenants/managed-first/connections", fundsBalanceTestPath} {
					resources[path] = exchange(http.MethodGet, path, "", http.StatusOK)
				}
				return resources
			}
			revision := uint64(1)
			if state == hostedGrantActive {
				exchange(http.MethodPatch, grantTransitionRecoveryPath, `{"state":"suspended","revision":1,"reason":"Prepare reactivation"}`, http.StatusOK)
				revision++
			}
			before := snapshot()
			body := fmt.Sprintf(`{"state":%q,"revision":%d,"reason":"Recover transition"}`, state, revision)
			var failedReads atomic.Int64
			const callbackName = "grant-transition-read-failure"
			if err := database.database.Callback().Query().After("gorm:query").Register(callbackName, func(transaction *gorm.DB) {
				_, inTransaction := transaction.Statement.ConnPool.(gorm.TxCommitter)
				if transaction.Statement.Table != "managed_hosted_grant_records" || !inTransaction {
					return
				}
				record, ok := transaction.Statement.Dest.(*managedHostedGrantRecord)
				if ok && record.Revision == revision+1 && record.State == state {
					failedReads.Add(1)
				}
				transaction.AddError(errInternalTestDatabase)
			}); err != nil {
				t.Fatal(err)
			}
			for range 2 {
				response := exchange(http.MethodPatch, grantTransitionRecoveryPath, body, http.StatusInternalServerError)
				if !reflect.DeepEqual(response, map[string]any{"error": map[string]any{"code": "hosted_access_store_failed"}}) {
					t.Fatalf("transition failure exposed partial or private data: %v", response)
				}
				if !reflect.DeepEqual(before, snapshot()) {
					t.Fatal("failed transition changed grant history, assignment, or funds")
				}
			}
			if failedReads.Load() != 2 {
				t.Fatalf("failed reads after grant mutation=%d want=2", failedReads.Load())
			}
			if err := database.database.Callback().Query().Remove(callbackName); err != nil {
				t.Fatal(err)
			}
			server.Close()
			server, cookie = newFundsManagementHTTPFixture(t, openJournalTransactionInstance(t, database))
			if !reflect.DeepEqual(before, snapshot()) {
				t.Fatal("restart changed the rolled-back transition")
			}
			accepted := exchange(http.MethodPatch, grantTransitionRecoveryPath, body, http.StatusOK)
			if accepted["state"] != string(state) || accepted["revision"] != float64(revision+1) {
				t.Fatalf("recovered transition=%v", accepted)
			}
			after := snapshot()
			prior := before[grantTransitionRecoveryPath+"/revisions"].(map[string]any)["revisions"].([]any)
			history := after[grantTransitionRecoveryPath+"/revisions"].(map[string]any)["revisions"].([]any)
			if len(history) != len(prior)+1 || !reflect.DeepEqual(history[:len(prior)], prior) {
				t.Fatalf("recovery changed prior revisions or repeated a transition: %v", history)
			}
			last := history[len(prior)].(map[string]any)
			if last["state"] != string(state) || last["revision"] != float64(revision+1) || last["actor_user_id"] != "operator" || last["reason"] != "Recover transition" {
				t.Fatalf("recovered audit=%v", last)
			}
			if !reflect.DeepEqual(before[fundsBalanceTestPath], after[fundsBalanceTestPath]) || !reflect.DeepEqual(before["/tenants/managed-first/connections"], after["/tenants/managed-first/connections"]) {
				t.Fatal("recovered grant transition changed assignment or funds")
			}
			for range 2 {
				server.Close()
				server, cookie = newFundsManagementHTTPFixture(t, openJournalTransactionInstance(t, database))
				exchange(http.MethodPatch, grantTransitionRecoveryPath, body, http.StatusConflict)
				if !reflect.DeepEqual(after, snapshot()) {
					t.Fatal("restart or stale revision retry changed the accepted transition")
				}
			}
		})
	}
}
