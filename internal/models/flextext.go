package models

import (
	"encoding/json"
	"fmt"
	"strings"
)

// FlexText is a string that tolerates a model returning structured text.
//
// A prompt asking for "bullet points" invites an array of bullets, and Gemini
// duly returns one — which made the whole analysis fail with "cannot unmarshal
// array into Go struct field". Rejecting a well-formed, useful answer over its
// container type is the wrong trade: the text is what matters, and the model's
// choice of shape is not something a prompt can guarantee.
//
// Marshals back out as a plain string, so the API shape is unchanged.
type FlexText string

// UnmarshalJSON accepts a string, an array, a number, or an object, and
// renders whatever it finds as text.
func (f *FlexText) UnmarshalJSON(data []byte) error {
	var direct string
	if err := json.Unmarshal(data, &direct); err == nil {
		*f = FlexText(direct)
		return nil
	}

	var loose any
	if err := json.Unmarshal(data, &loose); err != nil {
		// Genuinely malformed JSON is still an error; tolerance applies to
		// shape, not to garbage.
		return err
	}

	*f = FlexText(flatten(loose))
	return nil
}

// String makes FlexText printable without a conversion at every call site.
func (f FlexText) String() string { return string(f) }

// flatten renders an arbitrary decoded value as readable text.
//
// Arrays become one line per element, because that is what a model returning
// an array of bullet points meant. Objects contribute their values, since the
// keys are the model's own scaffolding ("point", "detail") rather than content
// a reader wants to see.
func flatten(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case bool:
		return fmt.Sprintf("%t", v)
	case float64:
		// Render whole numbers without a trailing .0, which is how they were
		// almost certainly written.
		if v == float64(int64(v)) {
			return fmt.Sprintf("%d", int64(v))
		}
		return fmt.Sprintf("%g", v)
	case []any:
		lines := make([]string, 0, len(v))
		for _, item := range v {
			if text := strings.TrimSpace(flatten(item)); text != "" {
				lines = append(lines, text)
			}
		}
		return strings.Join(lines, "\n")
	case map[string]any:
		// Sorted for stable output: map iteration order would otherwise make
		// the same response render differently between runs.
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sortStrings(keys)

		parts := make([]string, 0, len(v))
		for _, key := range keys {
			if text := strings.TrimSpace(flatten(v[key])); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, " ")
	default:
		return fmt.Sprintf("%v", v)
	}
}

// sortStrings avoids pulling in sort for one call site in a hot-ish path.
func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}
