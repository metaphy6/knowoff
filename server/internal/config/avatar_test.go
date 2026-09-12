package config

import "testing"

func TestAvatarScreeningCapabilityRequiresImageProviderAndSecret(t *testing.T) {
	valid := ContentScreeningConfig{Provider: "openai", Model: "omni-moderation-latest", APIKey: "fixture"}
	if !AvatarScreeningEnabled(valid) {
		t.Fatal("configured image capability closed")
	}
	for _, c := range []ContentScreeningConfig{{}, {Provider: "disabled"}, {Provider: "openai", Model: valid.Model}, {Provider: "openai", Model: "text-moderation-latest", APIKey: "fixture"}, {Provider: "other", Model: valid.Model, APIKey: "fixture"}, {Provider: "openai", Model: valid.Model, APIKey: "fixture", TimeoutS: 31}} {
		if AvatarScreeningEnabled(c) {
			t.Fatal("invalid image capability opened")
		}
	}
}
