package units

import (
	"strings"
	"unicode/utf8"

	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/icons"
)

func canonicalFieldSetting[T any](setting FieldSetting[T], validate func(T) error) (FieldSetting[T], error) {
	if setting.Mode == "" {
		setting.Mode = "inherit"
	}
	switch setting.Mode {
	case "inherit", "unset":
		if setting.Value != nil {
			return FieldSetting[T]{}, authz.ErrInvalidInput
		}
	case "value":
		if setting.Value == nil {
			return FieldSetting[T]{}, authz.ErrInvalidInput
		}
		value := *setting.Value
		if err := validate(value); err != nil {
			return FieldSetting[T]{}, err
		}
		setting.Value = &value
	default:
		return FieldSetting[T]{}, authz.ErrInvalidInput
	}
	return setting, nil
}

func canonicalDefaultSetting[T any](setting DefaultSetting[T], validate func(T) error) (DefaultSetting[T], error) {
	switch setting.Mode {
	case "unset":
		if setting.Value != nil {
			return DefaultSetting[T]{}, authz.ErrInvalidInput
		}
	case "value":
		if setting.Value == nil {
			return DefaultSetting[T]{}, authz.ErrInvalidInput
		}
		value := *setting.Value
		if err := validate(value); err != nil {
			return DefaultSetting[T]{}, err
		}
		setting.Value = &value
	default:
		return DefaultSetting[T]{}, authz.ErrInvalidInput
	}
	return setting, nil
}

func canonicalColor(value string) error {
	if len(value) != 7 || value[0] != '#' {
		return authz.ErrInvalidInput
	}
	for _, character := range value[1:] {
		if (character < '0' || character > '9') &&
			(character < 'a' || character > 'f') &&
			(character < 'A' || character > 'F') {
			return authz.ErrInvalidInput
		}
	}
	return nil
}

func canonicalIcon(value string) error {
	if !icons.Valid(value) {
		return authz.ErrInvalidInput
	}
	return nil
}

func canonicalFunctionMeanings(value map[int]string) error {
	if len(value) > 4 {
		return authz.ErrInvalidInput
	}
	for function, label := range value {
		if function < 0 || function > 3 || !utf8.ValidString(label) ||
			strings.TrimSpace(label) == "" || utf8.RuneCountInString(label) > 256 {
			return authz.ErrInvalidInput
		}
	}
	return nil
}

func copyFunctionMeanings(value map[int]string) map[int]string {
	if value == nil {
		return map[int]string{}
	}
	copied := make(map[int]string, len(value))
	for function, label := range value {
		copied[function] = label
	}
	return copied
}

// Canonical validates and copies all setting values, normalizing colors for storage.
func (settings Settings) Canonical() (Settings, error) {
	color, err := canonicalFieldSetting(settings.Color, canonicalColor)
	if err != nil {
		return Settings{}, err
	}
	if color.Value != nil {
		value := strings.ToUpper(*color.Value)
		color.Value = &value
	}
	icon, err := canonicalFieldSetting(settings.Icon, canonicalIcon)
	if err != nil {
		return Settings{}, err
	}
	meanings, err := canonicalFieldSetting(settings.FunctionMeanings, canonicalFunctionMeanings)
	if err != nil {
		return Settings{}, err
	}
	if meanings.Value != nil {
		value := copyFunctionMeanings(*meanings.Value)
		meanings.Value = &value
	}
	return Settings{Color: color, Icon: icon, FunctionMeanings: meanings}, nil
}

// Canonical validates complete Organization defaults and copies their values.
func (defaults Defaults) Canonical() (Defaults, error) {
	color, err := canonicalDefaultSetting(defaults.Color, canonicalColor)
	if err != nil {
		return Defaults{}, err
	}
	if color.Value != nil {
		value := strings.ToUpper(*color.Value)
		color.Value = &value
	}
	icon, err := canonicalDefaultSetting(defaults.Icon, canonicalIcon)
	if err != nil {
		return Defaults{}, err
	}
	meanings, err := canonicalDefaultSetting(defaults.FunctionMeanings, canonicalFunctionMeanings)
	if err != nil {
		return Defaults{}, err
	}
	if meanings.Value != nil {
		value := copyFunctionMeanings(*meanings.Value)
		meanings.Value = &value
	}
	return Defaults{Color: color, Icon: icon, FunctionMeanings: meanings}, nil
}

func effectiveValue[T any](setting FieldSetting[T], fallback DefaultSetting[T], hasOrganization bool) EffectiveValue[T] {
	switch setting.Mode {
	case "value":
		return EffectiveValue[T]{Value: setting.Value, Source: "unit"}
	case "unset":
		return EffectiveValue[T]{Source: "unset"}
	case "inherit":
		if !hasOrganization || fallback.Mode == "unset" {
			return EffectiveValue[T]{Source: "unset"}
		}
		return EffectiveValue[T]{Value: fallback.Value, Source: "organization"}
	default:
		return EffectiveValue[T]{Source: "unset"}
	}
}

func materializeSetting[T any](setting FieldSetting[T], fallback DefaultSetting[T]) FieldSetting[T] {
	if setting.Mode != "inherit" {
		return setting
	}
	if fallback.Mode == "value" {
		return FieldSetting[T]{Mode: "value", Value: fallback.Value}
	}
	return FieldSetting[T]{Mode: "unset"}
}

// ResolveSettings computes effective values without changing stored Unit overrides.
func ResolveSettings(settings Settings, defaults *Defaults) EffectiveSettings {
	var org Defaults
	hasOrganization := defaults != nil
	if hasOrganization {
		org = *defaults
	}
	return EffectiveSettings{
		Color:            effectiveValue(settings.Color, org.Color, hasOrganization),
		Icon:             effectiveValue(settings.Icon, org.Icon, hasOrganization),
		FunctionMeanings: effectiveValue(settings.FunctionMeanings, org.FunctionMeanings, hasOrganization),
	}
}

// MaterializeSettings replaces inherited values with the current Organization value or unset.
func MaterializeSettings(settings Settings, defaults Defaults) Settings {
	result := Settings{
		Color:            materializeSetting(settings.Color, defaults.Color),
		Icon:             materializeSetting(settings.Icon, defaults.Icon),
		FunctionMeanings: materializeSetting(settings.FunctionMeanings, defaults.FunctionMeanings),
	}
	if result.FunctionMeanings.Value != nil {
		value := copyFunctionMeanings(*result.FunctionMeanings.Value)
		result.FunctionMeanings.Value = &value
	}
	return result
}
