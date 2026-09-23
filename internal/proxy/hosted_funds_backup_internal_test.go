package proxy

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
	"time"
)

func fundsBackupView(t *testing.T, server *httptest.Server, cookie *http.Cookie, path string) map[string]any {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, server.URL+managementAPIPath+"/billing-accounts/billing-journal"+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.AddCookie(cookie)
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("restored %s status=%d body=%s", path, response.StatusCode, payload)
	}
	var value map[string]any
	if err := json.Unmarshal(payload, &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func copyFundsDatabase(t *testing.T, source, destination string) {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(t.Context(), "make", "--silent", "snapshot-managed-database")
	command.Dir = root
	command.Env = append(os.Environ(), "SNAPSHOT_SOURCE="+source, "SNAPSHOT_DESTINATION="+destination)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("database snapshot: %v output=%s", err, output)
	}
	var receipt struct {
		SHA256    string `json:"sha256"`
		SizeBytes int64  `json:"size_bytes"`
	}
	if err := json.Unmarshal(output, &receipt); err != nil || len(receipt.SHA256) != 64 || receipt.SizeBytes <= 0 {
		t.Fatalf("snapshot receipt=%s error=%v", output, err)
	}
}

func TestHostedFundsBackupRestoresFinancialEvidence(t *testing.T) {
	database, management, cookie, reservation := newFundsResolutionFixtureFor(t, `{"input_tokens":20000,"output_tokens":100,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}`, journalRequestCompleted)
	path := "/billing-accounts/billing-journal/requests/" + reservation.RequestID + "/funds-resolution"
	body := fmt.Sprintf(`{"revision":%d,"customer_charge":{"numerator":"3","denominator":"200"},"reason":"approved_exception","evidence_reference":"backup-audit"}`, reservation.Revision)
	fundsResolutionHTTP(t, management, cookie("operator"), http.MethodPut, path, body, http.StatusOK)
	var calls atomic.Int64
	upstream := fundsUpstream(t, &calls)
	generation := newHostedIdentityHTTPServer(t, database, upstream.URL, t.TempDir(), fundsDependencies(hostedRatingFixtureAdmission(t, 2)))
	for index := range 3 {
		hostedIdentityHTTP(t, generation, fmt.Sprintf("backup-funded-%d", index), "funded prompt", http.StatusOK)
		if err := deliverFundsFixture(t, database); err != nil {
			t.Fatal(err)
		}
	}
	var charges []managedChargeRecord
	if err := database.database.Where("state = ?", chargeRated).Order("id").Find(&charges).Error; err != nil || len(charges) != 3 {
		t.Fatalf("snapshot charges=%v error=%v", charges, err)
	}
	if err := applyFundsFixtureCredit(t, database, fundsCreditCommand(t, charges[0], "before-backup-credit")); err != nil {
		t.Fatal(err)
	}
	unknown := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodDelete {
			writer.WriteHeader(http.StatusNoContent)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"id":"unknown-backup","status":"completed","output_text":"funded result","usage":{}}`)
	}))
	t.Cleanup(unknown.Close)
	uncertain := newHostedIdentityHTTPServer(t, database, unknown.URL, t.TempDir(), fundsDependencies(hostedRatingFixtureAdmission(t, 2)))
	hostedIdentityHTTP(t, uncertain, "backup-uncertain", "funded prompt", http.StatusOK)
	if err := deliverFundsFixture(t, database); err != nil {
		t.Fatal(err)
	}
	var hold managedFundsReservationRecord
	if err := database.database.Where("state = ?", fundsReservationReconciliation).First(&hold).Error; err != nil {
		t.Fatal(err)
	}
	correctionPath := "/billing-accounts/billing-journal/requests/" + reservation.RequestID + "/funds-credits/backup-credit"
	fundsResolutionHTTP(t, management, cookie("operator"), http.MethodPut, correctionPath, `{"credit":{"numerator":"1","denominator":"10000"},"reason":"approved_correction","evidence_reference":"backup-review"}`, http.StatusOK)
	assertHostedFundsBalance(t, database, 3, 0)
	assertFundsCreditRemainder(t, database, "109", "50000")
	paths := []string{"/requests/" + reservation.RequestID + "/funds-credits/backup-credit", "/balance", "/reservations?limit=100", "/reservations/" + reservation.RequestID, "/ledger-entries?limit=100", "/charges?limit=100", "/requests?limit=100", "/tenant-limits/managed-first", "/requests/" + reservation.RequestID + "/funds-resolution", "/requests/" + reservation.RequestID + "/reconciliation-cases", "/requests/" + hold.RequestID + "/reconciliation-cases", "/requests/" + hold.RequestID + "/observations"}
	before := make(map[string]map[string]any, len(paths))
	for _, path := range paths {
		before[path] = fundsBackupView(t, management, cookie("owner"), path)
	}
	var source struct{ File string }
	if err := database.database.Raw("SELECT file FROM pragma_database_list WHERE name = 'main'").Scan(&source).Error; err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	backup, restoredPath := filepath.Join(root, "backup.db"), filepath.Join(root, "restored.db")
	copyFundsDatabase(t, source.File, backup)
	if err := applyFundsFixtureCredit(t, database, fundsCreditCommand(t, charges[1], "after-backup-credit")); err != nil {
		t.Fatal(err)
	}
	assertHostedFundsBalance(t, database, 4, 1)
	copyFundsDatabase(t, backup, restoredPath)
	restored, err := newGORMManagedTenantDatabase(ManagementConfiguration{DatabasePath: restoredPath}, internalManagedProviderKeyCipher(), internalManagementProviderRegistry())
	if err != nil {
		t.Fatal(err)
	}
	connection, err := restored.database.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := connection.Close(); err != nil {
			t.Error(err)
		}
	})
	for range 2 {
		if err := restored.reconcileHostedFunds(t.Context(), time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	restoredServer, restoredCookie := newFundsManagementHTTPFixture(t, restored)
	for _, path := range paths {
		actual := fundsBackupView(t, restoredServer, restoredCookie("owner"), path)
		if !reflect.DeepEqual(before[path], actual) {
			t.Fatalf("restored %s differs: before=%v after=%v", path, before[path], actual)
		}
	}
	assertHostedFundsBalance(t, restored, 3, 0)
	assertFundsCreditRemainder(t, restored, "109", "50000")
	var exposure managedFundsExposureRecord
	if err := restored.database.First(&exposure).Error; err != nil {
		t.Fatal(err)
	}
	if exposure.ExcessNumerator != "37" || exposure.ExcessDenominator != "2500" {
		t.Fatalf("restored exposure=%+v", exposure)
	}
}
