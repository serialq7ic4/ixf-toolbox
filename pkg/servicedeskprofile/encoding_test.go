package servicedeskprofile

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestEncodeProfileMatchesDiscoverCanonicalEncoding(t *testing.T) {
	original := mustReadFixture(t, "profile-4003.json")
	overlay, err := DecodeOverlay(original)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := EncodeProfile(overlay)
	if err != nil {
		t.Fatal(err)
	}
	want, err := json.MarshalIndent(overlay, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	want = append(want, '\n')
	if !bytes.Equal(encoded, want) {
		t.Fatalf("EncodeProfile() differs from discover encoding\nwant:\n%s\ngot:\n%s", want, encoded)
	}
}

func TestEncodeProfileNormalizesNilSlicesWithoutMutatingInput(t *testing.T) {
	overlay, err := DecodeOverlay(mustReadFixture(t, "profile-4003.json"))
	if err != nil {
		t.Fatal(err)
	}
	overlay.Routing.Examples = nil
	overlay.Baseline.Fields = nil
	overlay.Fields = map[string]OverlayField{}
	encoded, err := EncodeProfile(overlay)
	if err != nil {
		t.Fatal(err)
	}
	if overlay.Routing.Examples != nil || overlay.Baseline.Fields != nil {
		t.Fatal("EncodeProfile mutated caller slices")
	}
	if !bytes.Contains(encoded, []byte(`"examples": []`)) || !bytes.Contains(encoded, []byte(`"fields": []`)) {
		t.Fatalf("EncodeProfile did not normalize nil slices: %s", encoded)
	}
}

func TestEncodeRegistrySortsAndRoundTrips(t *testing.T) {
	registry := Registry{
		SchemaVersion: RegistrySchemaVersion,
		UpdatedAt:     time.Date(2026, 11, 24, 1, 2, 3, 987, time.FixedZone("CST", 8*60*60)),
		Profiles: []RegistryEntry{
			{Name: "z-last", Path: "profiles/z-last/" + strings.Repeat("b", 64) + ".json", SHA256: strings.Repeat("b", 64)},
			{Name: "a-first", Path: "profiles/a-first/" + strings.Repeat("a", 64) + ".json", SHA256: strings.Repeat("a", 64)},
		},
	}
	encoded, err := EncodeRegistry(registry)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Index(encoded, []byte(`"a-first"`)) > bytes.Index(encoded, []byte(`"z-last"`)) {
		t.Fatalf("registry entries are not sorted: %s", encoded)
	}
	if !bytes.Contains(encoded, []byte(`"updatedAt": "2026-11-23T17:02:03Z"`)) || encoded[len(encoded)-1] != '\n' {
		t.Fatalf("registry timestamp/newline = %s", encoded)
	}
	decoded, err := DecodeRegistry(encoded)
	if err != nil || len(decoded.Profiles) != 2 {
		t.Fatalf("DecodeRegistry() = %+v, %v", decoded, err)
	}
}

func TestContentAddressedProfilePathValidatesInputs(t *testing.T) {
	hash := strings.Repeat("a", 64)
	got, err := ContentAddressedProfilePath("hardware-repair", hash)
	if err != nil || got != "profiles/hardware-repair/"+hash+".json" {
		t.Fatalf("ContentAddressedProfilePath() = %q, %v", got, err)
	}
	for _, input := range [][2]string{{"Bad Name", hash}, {"valid", strings.ToUpper(hash)}, {"valid", "short"}} {
		if _, err := ContentAddressedProfilePath(input[0], input[1]); err == nil {
			t.Fatalf("ContentAddressedProfilePath(%q, %q) succeeded", input[0], input[1])
		}
	}
}

func TestEncodeRegistryEmptyProfiles(t *testing.T) {
	registry := Registry{SchemaVersion: RegistrySchemaVersion, UpdatedAt: time.Now()}
	for _, profiles := range [][]RegistryEntry{nil, {}} {
		registry.Profiles = profiles
		encoded, err := EncodeRegistry(registry)
		if err != nil || !bytes.Contains(encoded, []byte(`"profiles": []`)) {
			t.Fatalf("empty Registry = %s, %v", encoded, err)
		}
	}
}

func TestRegistryChecksNonAdjacentPathAncestors(t *testing.T) {
	registry := Registry{SchemaVersion: RegistrySchemaVersion, UpdatedAt: time.Now()}
	for i, path := range []string{"profiles/a", "profiles/a-backup", "profiles/a/v1.json"} {
		registry.Profiles = append(registry.Profiles, RegistryEntry{
			Name: []string{"first", "second", "third"}[i], Path: path, SHA256: strings.Repeat("a", 64),
		})
	}
	if err := ValidateRegistry(registry); err == nil {
		t.Fatal("an indexed file is also the parent directory of another indexed file")
	}
}
