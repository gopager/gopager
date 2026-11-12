package units

import (
	"testing"
)

func TestSettingsCanonical(t *testing.T) {
	t.Parallel()
	color := "#aAbB10"
	meanings := map[int]string{0: "fire"}
	settings, err := (Settings{
		Color:            FieldSetting[string]{Mode: "value", Value: &color},
		Icon:             FieldSetting[string]{Mode: "inherit"},
		FunctionMeanings: FieldSetting[map[int]string]{Mode: "value", Value: &meanings},
	}).Canonical()
	if err != nil {
		t.Fatal(err)
	}
	if got := *settings.Color.Value; got != "#AABB10" {
		t.Fatalf("canonical color = %q, want #AABB10", got)
	}
	meanings[0] = "changed"
	if got := (*settings.FunctionMeanings.Value)[0]; got != "fire" {
		t.Fatalf("canonical meanings were not copied: %q", got)
	}
}

func TestSettingsCanonicalRejectsInvalidValues(t *testing.T) {
	t.Parallel()
	tests := []Settings{
		{Color: FieldSetting[string]{Mode: "value"}},
		{Color: FieldSetting[string]{Mode: "value", Value: stringValue("#12345G")}},
		{Icon: FieldSetting[string]{Mode: "unset", Value: stringValue("icon")}},
		{FunctionMeanings: FieldSetting[map[int]string]{Mode: "value", Value: mapValue(map[int]string{4: "unknown"})}},
	}
	for _, input := range tests {
		if _, err := input.Canonical(); err == nil {
			t.Fatalf("Canonical(%+v) succeeded, want invalid input", input)
		}
	}
}

func TestResolveAndMaterializeSettings(t *testing.T) {
	t.Parallel()
	unitColor := "#112233"
	orgColor := "#AABBCC"
	defaults := Defaults{
		Color:            DefaultSetting[string]{Mode: "value", Value: &orgColor},
		Icon:             DefaultSetting[string]{Mode: "unset"},
		FunctionMeanings: DefaultSetting[map[int]string]{Mode: "unset"},
	}
	settings := Settings{
		Color:            FieldSetting[string]{Mode: "inherit"},
		Icon:             FieldSetting[string]{Mode: "value", Value: &unitColor},
		FunctionMeanings: FieldSetting[map[int]string]{Mode: "unset"},
	}
	effective := ResolveSettings(settings, &defaults, nil)
	if effective.Color.Source != "organization" || effective.Color.Value == nil || *effective.Color.Value != orgColor {
		t.Fatalf("effective color = %+v", effective.Color)
	}
	if effective.Icon.Source != "unit" || effective.Icon.Value == nil || *effective.Icon.Value != unitColor {
		t.Fatalf("effective icon = %+v", effective.Icon)
	}
	if effective.FunctionMeanings.Source != "unset" || effective.FunctionMeanings.Value != nil {
		t.Fatalf("effective meanings = %+v", effective.FunctionMeanings)
	}

	materialized := MaterializeSettings(settings, defaults, nil)
	if materialized.Color.Mode != "value" || materialized.Color.Value == nil || *materialized.Color.Value != orgColor ||
		materialized.FunctionMeanings.Mode != "unset" {
		t.Fatalf("materialized settings = %+v", materialized)
	}
	if got := ResolveSettings(materialized, nil, nil); got.Color.Source != "unit" || *got.Color.Value != orgColor {
		t.Fatalf("materialized value did not survive detachment: %+v", got.Color)
	}
}

func stringValue(value string) *string { return &value }

func mapValue(value map[int]string) *map[int]string { return &value }
