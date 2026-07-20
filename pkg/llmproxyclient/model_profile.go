package llmproxyclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

const modelProfileSubject = "model_profile"

// ModelProfileReader reads the current JSON model-profile document from a configured path.
type ModelProfileReader func(path string) ([]byte, error)

type modelProfile struct {
	provider string
	model    string
}

func (config Config) currentModelProfile() (modelProfile, error) {
	profileBytes, readError := config.modelProfileReader(config.modelProfilePath)
	if readError != nil {
		return modelProfile{}, fmt.Errorf(
			"%w: read %s path=%q: %v",
			ErrInvalidModelProfile,
			modelProfileSubject,
			config.modelProfilePath,
			readError,
		)
	}
	return decodeModelProfile(config.modelProfilePath, profileBytes)
}

func decodeModelProfile(profilePath string, profileBytes []byte) (modelProfile, error) {
	if !utf8.Valid(profileBytes) {
		return modelProfile{}, modelProfileSchemaError(profilePath, "document must use valid UTF-8")
	}
	decoder := json.NewDecoder(bytes.NewReader(profileBytes))
	openingToken, openingError := decoder.Token()
	if openingError != nil {
		return modelProfile{}, modelProfileDecodeError(profilePath, openingError)
	}
	openingDelimiter, isDelimiter := openingToken.(json.Delim)
	if !isDelimiter || openingDelimiter != '{' {
		return modelProfile{}, modelProfileSchemaError(profilePath, "document must be an object")
	}

	profileValues := map[string]string{}
	for decoder.More() {
		profileFieldToken, fieldError := decoder.Token()
		if fieldError != nil {
			return modelProfile{}, modelProfileDecodeError(profilePath, fieldError)
		}
		profileField := fmt.Sprint(profileFieldToken)
		if !isModelProfileField(profileField) {
			return modelProfile{}, modelProfileSchemaError(profilePath, fmt.Sprintf("unsupported field=%q", profileField))
		}
		if _, exists := profileValues[profileField]; exists {
			return modelProfile{}, modelProfileSchemaError(profilePath, fmt.Sprintf("duplicate field=%q", profileField))
		}
		var profileValue string
		if valueError := decoder.Decode(&profileValue); valueError != nil {
			return modelProfile{}, modelProfileDecodeError(profilePath, valueError)
		}
		profileValues[profileField] = profileValue
	}

	_, closingError := decoder.Token()
	if closingError != nil {
		return modelProfile{}, modelProfileDecodeError(profilePath, closingError)
	}
	if trailingError := decoder.Decode(&struct{}{}); trailingError != io.EOF {
		if trailingError != nil {
			return modelProfile{}, modelProfileDecodeError(profilePath, trailingError)
		}
		return modelProfile{}, modelProfileSchemaError(profilePath, "document must contain one JSON value")
	}
	return newModelProfile(profilePath, profileValues[queryProvider], profileValues[queryModel])
}

func isModelProfileField(profileField string) bool {
	switch profileField {
	case queryProvider, queryModel:
		return true
	default:
		return false
	}
}

func newModelProfile(profilePath string, provider string, model string) (modelProfile, error) {
	trimmedProvider := strings.TrimSpace(provider)
	if trimmedProvider == "" {
		return modelProfile{}, modelProfileSchemaError(profilePath, "missing provider")
	}
	trimmedModel := strings.TrimSpace(model)
	if trimmedModel == "" {
		return modelProfile{}, modelProfileSchemaError(profilePath, "missing model")
	}
	return modelProfile{provider: trimmedProvider, model: trimmedModel}, nil
}

func modelProfileDecodeError(profilePath string, decodeError error) error {
	return fmt.Errorf(
		"%w: decode %s path=%q: %v",
		ErrInvalidModelProfile,
		modelProfileSubject,
		profilePath,
		decodeError,
	)
}

func modelProfileSchemaError(profilePath string, message string) error {
	return fmt.Errorf(
		"%w: validate %s path=%q: %s",
		ErrInvalidModelProfile,
		modelProfileSubject,
		profilePath,
		message,
	)
}
