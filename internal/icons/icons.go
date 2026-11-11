// Package icons holds the icon identifier rule shared by Unit settings, Attribute-schema fields
// and Layout blocks. An icon is a presentation hint that a client maps to a glyph; the server
// never renders it.
package icons

// MaxLength is the maximum length of an icon identifier in bytes.
const MaxLength = 128

// Valid reports whether value is an icon identifier: 1 to MaxLength ASCII letters, digits,
// '.', '_' or '-'.
func Valid(value string) bool {
	if value == "" || len(value) > MaxLength {
		return false
	}
	for _, character := range value {
		if (character < 'a' || character > 'z') &&
			(character < 'A' || character > 'Z') &&
			(character < '0' || character > '9') &&
			character != '.' && character != '_' && character != '-' {
			return false
		}
	}
	return true
}
