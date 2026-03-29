package etcdstorage

import (
	"context"
	"errors"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

var (
	ErrKeyNotFound = errors.New("key not found")
)

type Storage struct {
	cli     *clientv3.Client
	timeout time.Duration
}

// New Конструктор
func New(etcdClient *clientv3.Client, timeout time.Duration) *Storage {
	return &Storage{cli: etcdClient, timeout: timeout}
}

// Put — запись значения
func (s *Storage) Put(ctx context.Context, key, value string) error {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	_, err := s.cli.Put(ctx, key, value)
	return err
}

// Get — получение значения
func (s *Storage) Get(ctx context.Context, key string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	resp, err := s.cli.Get(ctx, key)
	if err != nil {
		return "", err
	}

	if len(resp.Kvs) == 0 {
		return "", ErrKeyNotFound
	}

	return string(resp.Kvs[0].Value), nil
}

// Delete — удаление ключа
func (s *Storage) Delete(ctx context.Context, key string) error {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	_, err := s.cli.Delete(ctx, key)
	return err
}
