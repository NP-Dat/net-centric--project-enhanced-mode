package network

import "encoding/json"

// EncodeJSON marshals an interface{} into a JSON byte slice.
func EncodeJSON(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

// DecodeJSON unmarshals a JSON byte slice into an interface{}.
// The second argument should be a pointer to the struct you want to decode into.
func DecodeJSON(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}
