package proxy

import "time"

const journalRequestQuoteReserved journalRequestState = "quote_reserved"
const quoteIDPrefix = "quote-"
const quoteReserved = "reserved"
const quoteConsumed = "consumed"
const quoteExpired = "expired"
const quoteReleased = "released"
const quoteLifetime = 5 * time.Minute

type managedFinancialQuoteRecord struct {
	ID               string                      `gorm:"primaryKey"`
	TenantID         string                      `gorm:"not null;uniqueIndex:idx_quote_tenant_key"`
	Tenant           managedTenantRecord         `gorm:"foreignKey:TenantID;references:TenantID;constraint:OnDelete:RESTRICT"`
	KeyDigest        string                      `gorm:"not null;uniqueIndex:idx_quote_tenant_key"`
	RequestID        string                      `gorm:"not null;uniqueIndex"`
	Request          managedJournalRequestRecord `gorm:"foreignKey:RequestID;references:ID;constraint:OnDelete:RESTRICT"`
	Kind             string                      `gorm:"not null;check:kind IN ('native_v2','media_operation')"`
	ProcessingDigest string                      `gorm:"not null"`
	RequestDigest    string                      `gorm:"not null"`
	State            string                      `gorm:"not null;check:state IN ('reserved','consumed','expired','released')"`
	Revision         uint64                      `gorm:"not null;check:revision > 0"`
	ExpiresAt        time.Time                   `gorm:"not null"`
	CreatedAt        time.Time                   `gorm:"not null"`
	UpdatedAt        time.Time                   `gorm:"not null"`
}
