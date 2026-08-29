package credentials

import (
	"context"
	"errors"
	"time"
)

// StaticDevelopmentBroker is an explicit development-only credential source.
// Production composition must use a workload or customer-managed source.
type StaticDevelopmentBroker struct {
	credential Credential
	now        func() time.Time
}

func NewStaticDevelopmentBroker(values map[string][]byte, expiresAt time.Time) (*StaticDevelopmentBroker, error) {
	credential := New(values, expiresAt)
	if !credential.ValidAt(time.Now()) {
		return nil, errors.New("development credential must be non-empty and unexpired")
	}
	return &StaticDevelopmentBroker{credential: credential, now: time.Now}, nil
}

func (b *StaticDevelopmentBroker) Acquire(_ context.Context, request Request) (Credential, error) {
	if err := request.Validate(); err != nil {
		return Credential{}, err
	}
	if !b.credential.ValidAt(b.now()) {
		return Credential{}, errors.New("development credential expired")
	}
	return b.credential.Clone(), nil
}
