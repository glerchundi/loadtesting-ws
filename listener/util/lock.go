package util

import (
	"time"

	"github.com/bsm/redis-lock"
)

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

func (dl *RedLock) Renew() error {
	_, err := dl.lock.Lock()
	return err
}