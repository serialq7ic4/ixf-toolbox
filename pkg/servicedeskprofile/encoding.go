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
	copyValue := registry
	copyValue.UpdatedAt = copyValue.UpdatedAt.UTC().Truncate(time.Second)
	copyValue.Profiles = append([]RegistryEntry(nil), registry.Profiles...)
	if copyValue.Profiles == nil {
		copyValue.Profiles = []RegistryEntry{}
	}
	sort.Slice(copyValue.Profiles, func(i, j int) bool { return copyValue.Profiles[i].Name < copyValue.Profiles[j].Name })
	if err := ValidateRegistry(copyValue); err != nil {
		return nil, err
	}
	encoded, err := json.MarshalIndent(copyValue, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode ServiceDesk registry: %w", err)
	}
	encoded = append(encoded, '\n')
	decoded, err := DecodeRegistry(encoded)
	if err != nil {
		return nil, fmt.Errorf("self-check encoded ServiceDesk registry: %w", err)
	}
	decoded.Profiles = append([]RegistryEntry(nil), decoded.Profiles...)
	sort.Slice(decoded.Profiles, func(i, j int) bool { return decoded.Profiles[i].Name < decoded.Profiles[j].Name })
	second, err := json.MarshalIndent(decoded, "", "  ")
	if err != nil || !bytes.Equal(encoded, append(second, '\n')) {
		return nil, fmt.Errorf("encoded ServiceDesk registry is not deterministic")
	}
	return encoded, nil
}
