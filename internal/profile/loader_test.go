package profile

import (
	"strings"
	"testing"

	"themisy/internal/domain"
)

func TestLoadProfilesAndCanonicalHash(t *testing.T) {
	profiles, err := LoadDir("../../examples/profiles")
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 2 {
		t.Fatalf("profiles=%d, want 2", len(profiles))
	}
	for _, profile := range profiles {
		if !strings.HasPrefix(profile.ContentHash, "sha256:") {
			t.Fatalf("profile %s has invalid hash %q", profile.Metadata.Name, profile.ContentHash)
		}
	}
}

func TestProfileDecodeRejectsUnsupportedSchemaAndUnknownFields(t *testing.T) {
	for name, data := range map[string]string{
		"unsupported schema": "apiVersion: execution.themisy.io/v2\nkind: ReleaseProfile\n",
		"unknown field":      "apiVersion: execution.themisy.io/v1alpha1\nkind: ReleaseProfile\nunknown: true\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Decode([]byte(data)); err == nil {
				t.Fatal("expected decode error")
			}
		})
	}
}

func TestProfileHashIgnoresCapabilityOrderAndSource(t *testing.T) {
	loaded, err := LoadFile("../../examples/profiles/ecs-canary-critical.yaml")
	if err != nil {
		t.Fatal(err)
	}
	changed := loaded
	changed.Source = "elsewhere"
	changed.ContentHash = ""
	changed.Spec.RequiredCapabilities = append([]domain.Capability(nil), loaded.Spec.RequiredCapabilities...)
	for left, right := 0, len(changed.Spec.RequiredCapabilities)-1; left < right; left, right = left+1, right-1 {
		changed.Spec.RequiredCapabilities[left], changed.Spec.RequiredCapabilities[right] = changed.Spec.RequiredCapabilities[right], changed.Spec.RequiredCapabilities[left]
	}
	hash, err := Hash(changed)
	if err != nil {
		t.Fatal(err)
	}
	if hash != loaded.ContentHash {
		t.Fatalf("hash changed: %s != %s", hash, loaded.ContentHash)
	}
}
