// Package duplicate implements channel-agnostic duplicate detection helpers.
package duplicate

import "strings"

// NormalizeText builds the deterministic comparison form of a free-text
// field: trim, case-fold, collapse whitespace runs.
//
// ponytail: no Unicode NFC normalization; add golang.org/x/text/unicode/norm
// if multibyte merchant-name variants become a real duplicate source.
func NormalizeText(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

// NormalizeReference trims a reference ID. Matching is otherwise exact so the
// transactions_reference_idx index stays usable.
func NormalizeReference(s string) string {
	return strings.TrimSpace(s)
}
