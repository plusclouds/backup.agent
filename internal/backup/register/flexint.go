package register

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// FlexInt unmarshals a JSON integer from either a bare number (1234) or a
// numeric string ("1234") — confirmed necessary against the real platform's
// registration response, which serializes nats.port as a string even though
// docs/backup.agent/registration.md's example shows a bare number. Encodes
// back out as a plain number.
type FlexInt int

// UnmarshalJSON accepts either a JSON number or a JSON string containing one.
func (f *FlexInt) UnmarshalJSON(data []byte) error {
	var n int
	if err := json.Unmarshal(data, &n); err == nil {
		*f = FlexInt(n)
		return nil
	}

	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("FlexInt: value is neither a number nor a numeric string: %s", data)
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return fmt.Errorf("FlexInt: %q is not a valid integer: %w", s, err)
	}
	*f = FlexInt(n)
	return nil
}

// MarshalJSON always encodes as a plain JSON number.
func (f FlexInt) MarshalJSON() ([]byte, error) {
	return json.Marshal(int(f))
}
