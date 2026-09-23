package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/tyemirov/llm-proxy/internal/proxy"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// This separate lane consumes actual sandbox scenario outcomes. It does not
// create purchases, simulate notifications, or claim browser acceptance.
func TestPaddleSandboxQualification(t *testing.T) {
	configPath, runID, expectationsPath := os.Getenv("PADDLE_SANDBOX_CONFIG"), os.Getenv("PADDLE_SANDBOX_RUN_ID"), os.Getenv("PADDLE_SANDBOX_EXPECTATIONS")
	if configPath == "" && runID == "" && expectationsPath == "" {
		t.Skip("actual Paddle sandbox qualification is a separate operator-selected lane")
	}
	report, err := qualifyPaddleSandbox(t.Context(), configPath, runID, expectationsPath)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Verified sandbox financial checkpoint: %s", encoded)
}

type sandboxExpectedReceipt struct {
	CreditCents        string `json:"credit_cents"`
	GrossCents         string `json:"gross_cents"`
	TaxCents           string `json:"tax_cents"`
	ReversedCents      string `json:"reversed_cents"`
	PendingRefundCents string `json:"pending_refund_cents"`
}

type sandboxExpectedOrder struct {
	OrderID          string                  `json:"order_id"`
	BillingAccountID string                  `json:"billing_account_id"`
	State            string                  `json:"state"`
	Receipt          *sandboxExpectedReceipt `json:"receipt"`
}

type sandboxExpectations struct {
	Orders []sandboxExpectedOrder `json:"orders"`
}

func qualifyPaddleSandbox(ctx context.Context, configPath, runID, expectationsPath string) (proxy.PaymentReconciliationReport, error) {
	var empty proxy.PaymentReconciliationReport
	if configPath == "" || runID == "" || expectationsPath == "" {
		return empty, fmt.Errorf("sandbox qualification requires config, run ID, and expectations")
	}
	configuration, err := loadRuntimeConfiguration(configPath)
	if err != nil {
		return empty, err
	}
	payments := configuration.Payments
	if payments == nil || payments.Environment != "sandbox" || payments.APIBaseURL != "" || !strings.HasPrefix(payments.ClientToken, "test_") {
		return empty, fmt.Errorf("sandbox qualification requires sandbox payments, a test token, and no API origin override")
	}
	expected, err := readSandboxExpectations(expectationsPath)
	if err != nil {
		return empty, err
	}
	if err := verifySandboxOrders(ctx, configuration, expected); err != nil {
		return empty, err
	}
	startedAt := time.Now().UTC()
	report, err := proxy.ReconcilePayments(ctx, configuration.Management, payments, runID)
	if err != nil {
		return empty, err
	}
	if report.State != "completed" || report.CompletedOrders != report.TotalOrders || report.Environment != "sandbox" {
		return empty, fmt.Errorf("sandbox reconciliation %s is incomplete", runID)
	}
	if report.CreatedAt.Before(startedAt) {
		return empty, fmt.Errorf("sandbox qualification requires a new reconciliation run ID for the current financial checkpoint")
	}
	items := make(map[string]proxy.PaymentReconciliationItem, len(report.Items))
	for _, item := range report.Items {
		items[item.OrderID] = item
	}
	for _, order := range expected.Orders {
		item, exists := items[order.OrderID]
		if !exists || item.BillingAccountID != order.BillingAccountID || item.EvidenceDigest == "" {
			return empty, fmt.Errorf("sandbox reconciliation %s lacks bound evidence for order %s", runID, order.OrderID)
		}
		if len(item.Differences) != 0 {
			return empty, fmt.Errorf("sandbox reconciliation %s has differences for order %s: %+v", runID, order.OrderID, item.Differences)
		}
	}
	if err := verifySandboxOrders(ctx, configuration, expected); err != nil {
		return empty, err
	}
	return report, nil
}

func readSandboxExpectations(path string) (sandboxExpectations, error) {
	var expected sandboxExpectations
	file, err := os.Open(path)
	if err != nil {
		return expected, fmt.Errorf("open sandbox expectations: %w", err)
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, 65537))
	if err != nil || len(content) > 65536 {
		return expected, fmt.Errorf("sandbox expectations must be readable and at most 65536 bytes")
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&expected); err != nil {
		return expected, fmt.Errorf("decode sandbox expectations: %w", err)
	}
	if decoder.Decode(new(any)) != io.EOF || len(expected.Orders) == 0 {
		return expected, fmt.Errorf("sandbox expectations require one bounded JSON object with nonempty orders")
	}
	seen := make(map[string]bool, len(expected.Orders))
	cents := regexp.MustCompile(`^(0|[1-9][0-9]*)$`)
	for _, order := range expected.Orders {
		if order.OrderID == "" || order.BillingAccountID == "" || seen[order.OrderID] {
			return expected, fmt.Errorf("sandbox expectations require unique orders with billing accounts")
		}
		seen[order.OrderID] = true
		switch order.State {
		case "created", "pending", "failed":
			if order.Receipt != nil {
				return expected, fmt.Errorf("unpaid sandbox order %s cannot expect a receipt", order.OrderID)
			}
		case "paid", "partially_refunded", "refunded", "disputed":
			if order.Receipt == nil {
				return expected, fmt.Errorf("funded sandbox order %s requires receipt expectations", order.OrderID)
			}
			for _, value := range []string{order.Receipt.CreditCents, order.Receipt.GrossCents, order.Receipt.TaxCents, order.Receipt.ReversedCents, order.Receipt.PendingRefundCents} {
				if !cents.MatchString(value) {
					return expected, fmt.Errorf("sandbox receipt amounts must be canonical nonnegative cent strings")
				}
			}
		default:
			return expected, fmt.Errorf("sandbox expectations contain an unsupported funding state")
		}
	}
	return expected, nil
}

func TestHostedPaymentsSandboxQualificationRejectsInvalidInputs(t *testing.T) {
	directory := t.TempDir()
	configurationText := strings.ReplaceAll(completeManagementYAML(), "/tmp/llm-proxy-test.sqlite", filepath.Join(directory, "sandbox.sqlite")) + `
payments:
  environment: sandbox
  client_token: test_fixture
  processor_account_id: processor-fixture
  supplier_id: supplier-fixture
  api_key: fixture-key
  webhook_secret: fixture-secret
  offers:
    - code: five
      price_id: pri_01hv8x2axb33yr5y238zfwcn5p
      funding_cents: 500
`
	configPath := writeTestConfig(t, directory, configurationText)
	configuration, err := loadRuntimeConfiguration(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := proxy.BuildRouter(configuration, zap.NewNop().Sugar()); err != nil {
		t.Fatal(err)
	}
	missingOrder := `{"orders":[{"order_id":"missing","billing_account_id":"missing","state":"pending","receipt":null}]}`
	for _, scenario := range []struct {
		name, config, expected, message string
	}{
		{"production", strings.NewReplacer("environment: sandbox", "environment: production", "test_fixture", "live_fixture").Replace(configurationText), missingOrder, "requires sandbox payments"},
		{"origin override", strings.Replace(configurationText, "payments:", "payments:\n  api_base_url: https://sandbox-api.paddle.com", 1), missingOrder, "no API origin override"},
		{"empty orders", configurationText, `{"orders":[]}`, "nonempty orders"},
		{"unknown field", configurationText, `{"orders":[],"passed":true}`, "unknown field"},
		{"trailing object", configurationText, missingOrder + `{}`, "one bounded JSON object"},
		{"oversized whitespace", configurationText, missingOrder + strings.Repeat(" ", 65536), "at most 65536 bytes"},
		{"missing receipt", configurationText, strings.Replace(missingOrder, "pending", "paid", 1), "requires receipt expectations"},
		{"unknown state", configurationText, strings.Replace(missingOrder, "pending", "success", 1), "unsupported funding state"},
		{"missing order", configurationText, missingOrder, "does not match its expected account"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			path := writeTestConfig(t, directory, scenario.config)
			expectedFile, err := os.CreateTemp(directory, "expectations-*.json")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := expectedFile.WriteString(scenario.expected); err != nil {
				t.Fatal(err)
			}
			if err := expectedFile.Close(); err != nil {
				t.Fatal(err)
			}
			if _, err := qualifyPaddleSandbox(t.Context(), path, "invalid-input-run", expectedFile.Name()); err == nil || !strings.Contains(err.Error(), scenario.message) {
				t.Fatalf("qualification error=%v; want %q", err, scenario.message)
			}
		})
	}
	if _, err := qualifyPaddleSandbox(t.Context(), "", "", ""); err == nil {
		t.Fatal("missing inputs passed qualification")
	}
}

func verifySandboxOrders(ctx context.Context, configuration proxy.Configuration, expected sandboxExpectations) error {
	if configuration.Management.DatabasePath == "" {
		return fmt.Errorf("sandbox qualification requires the managed SQLite database path")
	}
	absolutePath, err := filepath.Abs(configuration.Management.DatabasePath)
	if err != nil {
		return err
	}
	databaseURL := url.URL{Scheme: "file", Path: absolutePath, RawQuery: "mode=ro"}
	database, err := gorm.Open(sqlite.Open(databaseURL.String()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return fmt.Errorf("open sandbox evidence database: %w", err)
	}
	connection, err := database.DB()
	if err != nil {
		return err
	}
	defer connection.Close()
	return database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, order := range expected.Orders {
			var actual struct {
				State              string
				Currency           string
				ReceiptOrderID     *string
				CreditCents        string
				GrossCents         string
				TaxCents           string
				ReversedCents      string
				PendingRefundCents string
			}
			result := tx.Raw(`SELECT orders.state, orders.currency, receipt.order_id AS receipt_order_id,
CAST(receipt.credit_cents AS TEXT) AS credit_cents, receipt.gross_cents, receipt.tax_cents,
CAST(adjustment.reversed_cents AS TEXT) AS reversed_cents, CAST(adjustment.pending_cents AS TEXT) AS pending_refund_cents
FROM managed_funding_order_records AS orders
LEFT JOIN managed_payment_receipt_records AS receipt ON receipt.order_id = orders.id
LEFT JOIN managed_payment_adjustment_records AS adjustment ON adjustment.order_id = orders.id
WHERE orders.id = ? AND orders.billing_account_id = ? AND orders.environment = 'sandbox'
AND orders.processor_account_id = ? AND orders.supplier_id = ?`, order.OrderID, order.BillingAccountID,
				configuration.Payments.ProcessorAccountID, configuration.Payments.SupplierID).Scan(&actual)
			if result.Error != nil {
				return fmt.Errorf("read sandbox evidence for order %s: %w", order.OrderID, result.Error)
			}
			if result.RowsAffected != 1 || actual.State != order.State || actual.Currency != "USD" {
				return fmt.Errorf("sandbox order %s does not match its expected account, identity, currency, or state", order.OrderID)
			}
			var receipt *sandboxExpectedReceipt
			if actual.ReceiptOrderID != nil {
				receipt = &sandboxExpectedReceipt{actual.CreditCents, actual.GrossCents, actual.TaxCents, actual.ReversedCents, actual.PendingRefundCents}
			}
			if !reflect.DeepEqual(receipt, order.Receipt) {
				return fmt.Errorf("sandbox receipt amounts differ for order %s", order.OrderID)
			}
		}
		return nil
	})
}
