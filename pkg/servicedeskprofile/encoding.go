package servicedeskprofile

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

func EncodeProfile(overlay Overlay) ([]byte, error) {
	if err := ValidateOverlaySchema(overlay); err != nil {
		return nil, err
	}
	encoded, err := encodeProfileUnchecked(overlay)
	if err != nil {
		return nil, err
	}
	decoded, err := DecodeOverlay(encoded)
	if err != nil {
		return nil, fmt.Errorf("self-check encoded ServiceDesk profile: %w", err)
	}
	second, err := encodeProfileUnchecked(decoded)
	if err != nil || !bytes.Equal(encoded, second) {
		return nil, fmt.Errorf("encoded ServiceDesk profile is not deterministic")
	}
	return encoded, nil
}

func encodeProfileUnchecked(overlay Overlay) ([]byte, error) {
	copyValue := overlay
	if copyValue.Routing.Examples == nil {
		copyValue.Routing.Examples = []string{}
	}
	if copyValue.Baseline.Fields == nil {
		copyValue.Baseline.Fields = []BaselineField{}
	}
	encoded, err := json.MarshalIndent(copyValue, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode ServiceDesk profile: %w", err)
	}
	return append(encoded, '\n'), nil
}

func EncodeRegistry(registry Registry) ([]byte, error) {
	if registry.SchemaVersion != RegistrySchemaVersion {
		return nil, fmt.Errorf("ServiceDesk registry schemaVersion must be %d", RegistrySchemaVersion)
	}
	if registry.UpdatedAt.IsZero() {
		return nil, fmt.Errorf("ServiceDesk registry updatedAt is required")
	}
	if err := ValidateRegistry(registry); err != nil {
		return nil, err
	}
	encoded, err := encodeRegistryUnchecked(registry)
	if err != nil {
		return nil, fmt.Errorf("encode ServiceDesk registry: %w", err)
	}
	decoded, err := DecodeRegistry(encoded)
	if err != nil {
		return nil, fmt.Errorf("self-check encoded ServiceDesk registry: %w", err)
	}
	second, err := encodeRegistryUnchecked(decoded)
	if err != nil || !bytes.Equal(encoded, second) {
		return nil, fmt.Errorf("encoded ServiceDesk registry is not deterministic")
	}
	return encoded, nil
}

func encodeRegistryUnchecked(registry Registry) ([]byte, error) {
	registry.UpdatedAt = registry.UpdatedAt.UTC().Truncate(time.Second)
	registry.Profiles = append(make([]RegistryEntry, 0, len(registry.Profiles)), registry.Profiles...)
	sort.Slice(registry.Profiles, func(i, j int) bool { return registry.Profiles[i].Name < registry.Profiles[j].Name })
	encoded, err := json.MarshalIndent(registry, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(encoded, '\n'), nil
}
