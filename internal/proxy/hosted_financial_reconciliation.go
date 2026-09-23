package proxy

// FinancialReconciliationDifference identifies a difference between retained records and external financial evidence.
type FinancialReconciliationDifference struct {
	Category string `json:"category"`
	Code     string `json:"code"`
	Expected string `json:"expected"`
	Observed string `json:"observed"`
}
