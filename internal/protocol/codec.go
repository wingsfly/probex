package protocol

import "encoding/json"

// Encode serializes an envelope to bytes.
// Currently uses JSON; can be swapped to CBOR later.
func Encode(env Envelope) ([]byte, error) {
	return json.Marshal(env)
}

// Decode deserializes bytes into an envelope.
func Decode(data []byte) (Envelope, error) {
	var env Envelope
	err := json.Unmarshal(data, &env)
	return env, err
}

// NewEnvelope creates an envelope with the given type and payload.
func NewEnvelope(msgType MsgType, payload any) (Envelope, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return Envelope{}, err
	}
	return Envelope{Type: msgType, Payload: data}, nil
}

// DecodePayload unmarshals the payload into the given target.
func (e *Envelope) DecodePayload(target any) error {
	return json.Unmarshal(e.Payload, target)
}
