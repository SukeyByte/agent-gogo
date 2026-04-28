package provider

import (
	"context"
	"errors"
)

type MemoryStorageProvider struct {
	values map[string][]byte
}

func NewMemoryStorageProvider() *MemoryStorageProvider {
	return &MemoryStorageProvider{values: map[string][]byte{}}
}

func (p *MemoryStorageProvider) Put(ctx context.Context, key string, value []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	p.values[key] = append([]byte(nil), value...)
	return nil
}

func (p *MemoryStorageProvider) Get(ctx context.Context, key string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	value, ok := p.values[key]
	if !ok {
		return nil, errors.New("storage key not found")
	}
	return append([]byte(nil), value...), nil
}
