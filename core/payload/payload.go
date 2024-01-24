// Package payload defines payload that will be write to zipper.
package payload

// Payload is used to send data to zipper.
// Target is the target clientID of sfn.
type Payload struct {
	// Data is the data that will be sent to zipper.
	Data []byte
	// Target is the target clientID of sfn.
	Target string
	// TID is the TID of the payload.
	TID string
}

// New returns a new Payload from data.
func New(data []byte) *Payload {
	return &Payload{
		Data: data,
	}
}

// WithTarget returns a new Payload with target.
func (p *Payload) WithTarget(target string) *Payload {
	p.Target = target
	return p
}

// WithTID returns a new Payload with TID.
func (p *Payload) WithTID(TID string) *Payload {
	p.TID = TID
	return p
}
