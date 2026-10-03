package proxy

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/tyemirov/llm-proxy/pkg/llmproxyclient"
	"gorm.io/gorm/clause"
)

const alignmentOtherAccountKey = "alignment-other-account-key"
const alignmentOtherGrantID = "grant-22222222222222222222222222222222"

func seedHostedAlignmentOtherAccount(t *testing.T, database *gormManagedTenantDatabase, catalogRevision string) {
	t.Helper()
	now := time.Now().UTC()
	otherTenant := fakeTenantRecord("alignment-other", "alignment-other-tenant", "Other alignment customer", now)
	secretDigest := sha256Hex(alignmentOtherAccountKey)
	otherTenant.SecretDigest = &secretDigest
	for _, record := range []any{
		&managedUserRecord{UserID: "alignment-other", UserEmail: "alignment-other@example.com", CreatedAt: now, UpdatedAt: now},
		&otherTenant,
		&managedBillingAccountRecord{ID: "billing-alignment-other", OwnerUserID: "alignment-other", Currency: CatalogCurrencyUSD, CreationKeyDigest: sha256Hex("alignment-other-account"), CreatedAt: now},
		&managedFundsAccountRecord{BillingAccountID: "billing-alignment-other", State: fundsAccountActive, RemainderNumerator: "0", RemainderDenominator: "1", CreatedAt: now},
		&managedHostedGrantRecord{ID: alignmentOtherGrantID, BillingAccountID: "billing-alignment-other", TenantID: otherTenant.TenantID, PlatformConnectionID: "platform-service", Provider: "elevenlabs", CatalogRevision: catalogRevision, Offerings: []byte(`[{"operations":["audio_alignment"]}]`), State: hostedGrantActive, Revision: 1, CreatedAt: now, UpdatedAt: now},
		&managedHostedGrantRevisionRecord{GrantID: alignmentOtherGrantID, Revision: 1, State: hostedGrantActive, ActorUserID: "operator", Reason: "Alignment isolation acceptance", CreatedAt: now},
		&managedHostedTenantAssignmentRecord{TenantID: otherTenant.TenantID, ProviderID: "elevenlabs", GrantID: alignmentOtherGrantID, CreatedAt: now},
		&managedProviderProfileRecord{TenantID: otherTenant.TenantID, ProviderID: "elevenlabs", CreatedAt: now, UpdatedAt: now},
	} {
		if err := database.database.Omit(clause.Associations).Create(record).Error; err != nil {
			t.Fatal(err)
		}
	}
}

func assertHostedAlignmentAccountIsolation(t *testing.T, database *gormManagedTenantDatabase, baseURL string, operation map[string]any, inputAssetID string) {
	t.Helper()
	configuration, err := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{BaseURL: baseURL, Secret: alignmentOtherAccountKey})
	if err != nil {
		t.Fatal(err)
	}
	client, err := llmproxyclient.NewClient(configuration, http.DefaultClient)
	if err != nil {
		t.Fatal(err)
	}
	// A successful owned upload proves that the second customer's key is valid.
	ownedAsset, err := client.UploadAsset(t.Context(), llmproxyclient.AssetUploadInput{MIMEType: "audio/wav", Data: []byte("other customer audio")})
	if err != nil {
		t.Fatal(err)
	}
	requireStatus := func(err error, status int) {
		t.Helper()
		var failure *llmproxyclient.HTTPFailure
		if !errors.As(err, &failure) || failure.StatusCode() != status {
			t.Fatalf("other customer response=%v want status=%d", err, status)
		}
	}
	_, err = client.GetMediaOperation(t.Context(), operation["operation_id"].(string))
	requireStatus(err, http.StatusNotFound)
	_, err = client.CancelMediaOperation(t.Context(), operation["operation_id"].(string))
	requireStatus(err, http.StatusNotFound)
	_, err = client.GetAsset(t.Context(), inputAssetID)
	requireStatus(err, http.StatusNotFound)
	for _, output := range operation["outputs"].([]any) {
		_, err = client.GetAsset(t.Context(), output.(map[string]any)["asset_id"].(string))
		requireStatus(err, http.StatusNotFound)
	}
	_, err = client.CreateMediaOperation(t.Context(), "funded-alignment", llmproxyclient.MediaOperationInput{
		Capability: "audio.align", Provider: "elevenlabs",
		Input: json.RawMessage(fmt.Sprintf(`{"audio_asset_id":%q,"transcript":"Names"}`, ownedAsset.AssetID)), Controls: json.RawMessage(`{}`),
	})
	requireStatus(err, http.StatusPaymentRequired)

	server, cookie := newFundsManagementHTTPFixture(t, database)
	defer server.Close()
	contract, err := internalCanonicalOpenAPIContract()
	if err != nil {
		t.Fatal(err)
	}
	reader := financialReadFixture{server: server, cookie: cookie, contract: contract}
	for _, resource := range []string{"balance", "charges", "requests", "reservations", "ledger-entries"} {
		path := "/billing-accounts/billing-journal/" + resource
		view := financialReadResource{path: path, template: managementAPIPath + "/billing-accounts/{billing_account_id}/" + resource}
		before := reader.read(t, view, cookie("owner"), http.StatusOK)
		reader.read(t, view, cookie("alignment-other"), http.StatusNotFound)
		if after := reader.read(t, view, cookie("owner"), http.StatusOK); !reflect.DeepEqual(after, before) {
			t.Fatalf("other customer changed %s: %v", resource, after)
		}
	}
	otherBalance := financialReadResource{path: "/billing-accounts/billing-alignment-other/balance", template: managementAPIPath + "/billing-accounts/{billing_account_id}/balance"}
	reader.read(t, otherBalance, cookie("alignment-other"), http.StatusOK)
	reader.read(t, otherBalance, cookie("owner"), http.StatusNotFound)
}
