package proxy

import (
	"encoding/json"
	"fmt"
	"regexp"

	"gopkg.in/yaml.v3"
)

const catalogDecimalMaximumLength = 128

var catalogDecimalPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)(\.[0-9]*[1-9])?$`)

// CatalogDecimal is the canonical exact representation of a nonnegative amount.
// Catalog validation rejects its empty zero value and malformed direct inputs.
type CatalogDecimal string

// NewCatalogDecimal validates an exact amount without binary floating point.
func NewCatalogDecimal(value string) (CatalogDecimal, error) {
	if len(value) > catalogDecimalMaximumLength || !catalogDecimalPattern.MatchString(value) {
		return "", fmt.Errorf("%w: invalid exact decimal amount", ErrInvalidModelCatalog)
	}
	return CatalogDecimal(value), nil
}

// UnmarshalYAML accepts the current decimal string contract only.
func (amount *CatalogDecimal) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.ScalarNode || node.Tag != "!!str" {
		return fmt.Errorf("%w: monetary amount must be a decimal string", ErrInvalidModelCatalog)
	}
	value, err := NewCatalogDecimal(node.Value)
	if err != nil {
		return err
	}
	*amount = value
	return nil
}

// UnmarshalJSON retains exact amounts at public and persisted JSON boundaries.
func (amount *CatalogDecimal) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return fmt.Errorf("decode exact decimal amount: %w", err)
	}
	value, err := NewCatalogDecimal(text)
	if err != nil {
		return err
	}
	*amount = value
	return nil
}

func validCatalogDecimal(amount CatalogDecimal) bool {
	_, err := NewCatalogDecimal(string(amount))
	return err == nil
}
