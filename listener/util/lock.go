package util

import (
	"time"

	"github.com/bsm/redis-lock"
)


type Lock interface {
	Lock() (bool, error)
	Unlock() error
	Renew() (bool, error)
}

type RedLock struct {
	lock *lock.Lock
}

func NewRedLock(client lock.RedisClient, key string, ttl time.Duration) *RedLock {
	return &RedLock{
		lock: lock.NewLock(client, key, &lock.LockOptions{
			LockTimeout: ttl,
			WaitTimeout: 0,
		}),
	}
}

func (dl *RedLock) Lock() (bool, error) {
	return dl.lock.Lock()
}

func (dl *RedLock) Unlock() error {
	return dl.lock.Unlock()
}

func (dl *RedLock) Renew() (bool, error) {
	return dl.lock.Lock()
}

type MockLock struct {
}

func NewMockLock() *MockLock {
	return &MockLock{}
}

func (dl *MockLock) Lock() (bool, error) {
	return true, nil
}

func (dl *MockLock) Unlock() error {
	return nil
}

func (dl *MockLock) Renew() (bool, error) {
	return true, nil
}