package proxy

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"gorm.io/gorm"
)

func (request MediaOperationExecutionRequest) recordImageUsage(body []byte) error {
	if request.recordUsage == nil {
		return nil
	}
	input, _ := imageJournalMeter().observe(body, http.StatusOK, "", time.Time{})
	// The documented Images response has no separate cached modality counters.
	// Retain the gap instead of billing all input as uncached or inventing zeros.
	input.Quantities = append(input.Quantities, unknownImageCacheQuantities()...)
	return request.recordUsage(input)
}

func imageJournalMeter() journalTokenMeter {
	return journalTokenMeter{codec: CatalogProtocolOpenAIImages, fields: []journalMeterField{
		{"total_tokens", "usage.total_tokens", ""},
		{"input_tokens", "usage.input_tokens", "total_tokens"},
		{"output_tokens", "usage.output_tokens", "total_tokens"},
		{"input_text_tokens", "usage.input_tokens_details.text_tokens", "input_tokens"},
		{"input_image_tokens", "usage.input_tokens_details.image_tokens", "input_tokens"},
		{"output_text_tokens", "usage.output_tokens_details.text_tokens", "output_tokens"},
		{"output_image_tokens", "usage.output_tokens_details.image_tokens", "output_tokens"},
	}}
}

func unknownImageCacheQuantities() []journalQuantity {
	return []journalQuantity{
		{Dimension: "cache_read_text_tokens", Unit: "token", IncludedIn: "input_text_tokens", UnknownReason: journalQuantityUnsupported},
		{Dimension: "cache_read_image_tokens", Unit: "token", IncludedIn: "input_image_tokens", UnknownReason: journalQuantityUnsupported},
	}
}

// Responses reports model usage separately from its image-generation tool.
// Preserve the reported layer without inventing tool counters or cached splits.
func (request MediaOperationExecutionRequest) recordResponsesImageUsage(snapshot imageResponsesSnapshot) error {
	if request.recordUsage == nil {
		return nil
	}
	body, err := json.Marshal(struct {
		ID    string          `json:"id"`
		Usage json.RawMessage `json:"usage"`
	}{snapshot.ID, snapshot.Usage})
	if err != nil {
		return fmt.Errorf("encode response usage: %w", err)
	}
	profile := journalTokenMeter{codec: CatalogProtocolOpenAIResponses + ":image", fields: []journalMeterField{
		{"total_tokens", "usage.total_tokens", ""},
		{"input_tokens", "usage.input_tokens", "total_tokens"},
		{"output_tokens", "usage.output_tokens", "total_tokens"},
		{"cache_read_tokens", "usage.input_tokens_details.cached_tokens", "input_tokens"},
		{"cache_write_tokens", "usage.input_tokens_details.cache_write_tokens", "input_tokens"},
		{"reasoning_tokens", "usage.output_tokens_details.reasoning_tokens", "output_tokens"},
	}}
	input, _ := profile.observe(body, http.StatusOK, "", time.Time{})
	for index := range input.Quantities {
		quantity := &input.Quantities[index]
		quantity.Dimension = "responses_" + quantity.Dimension
		if quantity.IncludedIn != "" {
			quantity.IncludedIn = "responses_" + quantity.IncludedIn
		}
	}
	for _, field := range imageJournalMeter().fields {
		input.Quantities = append(input.Quantities, journalQuantity{Dimension: field.dimension, Unit: "token", IncludedIn: field.includedIn, UnknownReason: journalQuantityUnsupported})
	}
	input.Quantities = append(input.Quantities, unknownImageCacheQuantities()...)
	return request.recordUsage(input)
}

func (service *mediaOperationService) observeHostedMediaUsage(ctx context.Context, operationID string, generation uint64, input journalUsageEvidenceInput) error {
	persistContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), journalPersistenceTimeout)
	defer cancel()
	return service.store.database.WithContext(persistContext).Transaction(func(transaction *gorm.DB) error {
		now := service.store.now()
		if err := lockMediaOperationClaim(transaction, operationID, generation, now); err != nil {
			return err
		}
		request, err := service.store.hostedMediaRequest(transaction, operationID)
		if err != nil {
			return err
		}
		if request == nil {
			return errHostedAuthorityDenied
		}
		var attempt managedJournalAttemptRecord
		if err := transaction.Where("request_id = ?", request.ID).Order("number DESC").First(&attempt).Error; err != nil {
			return fmt.Errorf("read media usage attempt for operation %s: %w", operationID, err)
		}
		input.AttemptID, input.ObservedAt = attempt.ID, now
		if input.ProviderRequestID == "" {
			input.ProviderRequestID = attempt.ProviderRequestID
		}
		evidence, err := newJournalUsageEvidence(input, rand.Reader)
		if err != nil {
			return fmt.Errorf("normalize media usage for operation %s: %w", operationID, err)
		}
		journal := &gormManagedTenantDatabase{database: transaction}
		claim := journalWorkerClaim{requestID: request.ID, owner: mediaJournalWorkerToken(operationID, generation), now: now}
		_, err = journal.observeJournalAttempt(persistContext, claim, evidence)
		return err
	})
}
