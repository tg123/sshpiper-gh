package main

import (
	"time"

	"github.com/patrickmn/go-cache"
)

type sessionstore interface {
	GetSecret(session string) ([]byte, error)
	SetSecret(session string, secret []byte) error

	GetUpstream(session string) (upstream *upstreamConfig, err error)
	SetUpstream(session string, upstream *upstreamConfig) error

	SetSshError(session string, err string) error
	GetSshError(session string) (err *string)

	DeleteSession(session string, keeperr bool) error
}

type sessionstoreMemory struct {
	store *cache.Cache
}

func newSessionstoreMemory() (*sessionstoreMemory, error) {
	return &sessionstoreMemory{
		store: cache.New(1*time.Minute, 10*time.Minute),
	}, nil
}

func (s *sessionstoreMemory) GetSecret(session string) ([]byte, error) {
	secret, found := s.store.Get(session + "-secret")
	if !found {
		return nil, nil
	}

	return secret.([]byte), nil
}

func (s *sessionstoreMemory) SetSecret(session string, secret []byte) error {
	s.store.Set(session+"-secret", secret, cache.DefaultExpiration)
	return nil
}

func (s *sessionstoreMemory) GetUpstream(session string) (*upstreamConfig, error) {
	upstream, found := s.store.Get(session + "-upstream")
	if !found {
		return nil, nil
	}

	return upstream.(*upstreamConfig), nil
}

func (s *sessionstoreMemory) SetUpstream(session string, upstream *upstreamConfig) error {
	s.store.Set(session+"-upstream", upstream, cache.DefaultExpiration)
	return nil
}

func (s *sessionstoreMemory) SetSshError(session string, err string) error {
	s.store.Set(session+"-ssherror", &err, cache.DefaultExpiration)
	return nil
}

func (s *sessionstoreMemory) GetSshError(session string) (err *string) {
	ssherror, found := s.store.Get(session + "-ssherror")
	if !found {
		return nil
	}

	return ssherror.(*string)
}

func (s *sessionstoreMemory) DeleteSession(session string, keeperr bool) error {
	s.store.Delete(session + "-secret")
	s.store.Delete(session + "-upstream")
	if !keeperr {
		s.store.Delete(session + "-ssherror")
	}
	return nil
}
