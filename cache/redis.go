package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
)

type Redis struct {
	*redis.Client
	mu  sync.RWMutex
	mus map[string]*sync.RWMutex
}

func NewRedis(host string, port int, password string, db int) (*Redis, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", host, port),
		Password: password,
		DB:       db,
	})
	if _, err := client.Ping(context.TODO()).Result(); err != nil {
		return nil, err
	}
	return &Redis{
		Client: client,
		mus:    make(map[string]*sync.RWMutex),
	}, nil
	// defer client.Close()
}

func (c *Redis) gcRWMutex(key string) *sync.RWMutex {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.mus[key] == nil {
		c.mus[key] = &sync.RWMutex{}
	}
	return c.mus[key]
}

func (c *Redis) LockRun(id string, timeout time.Duration, fn func() error) error {
	if _, err := c.Client.SetNX(context.TODO(), id, "ok", timeout).Result(); err != nil {
		return fmt.Errorf("the system is busy. please try again later. id:%s", id)
	}
	defer c.Client.Del(context.TODO(), id)
	return fn()
}

func (c *Redis) Get(key string, result interface{}) error {
	rel, err := c.Client.Get(context.TODO(), key).Result()
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(rel), result)
}

func (c *Redis) Set(key string, create func() (*Item, error)) error {
	mu := c.gcRWMutex(key)
	mu.Lock()
	defer mu.Unlock()

	item, err := create()
	if err != nil {
		return err
	}
	body, err := json.Marshal(item.Value)
	if err != nil {
		return err
	}
	_, err = c.Client.Set(context.TODO(), key, body, item.Duration).Result()
	return err
}

func (c *Redis) GetOrSet(key string, result interface{}, create func() (*Item, error)) error {
	mu := c.gcRWMutex(key)
	mu.Lock()
	defer mu.Unlock()

	rel, err := c.Client.Get(context.TODO(), key).Result()
	if err == nil {
		return json.Unmarshal([]byte(rel), result)
	}

	item, err := create()
	if err != nil {
		return err
	}
	body, err := json.Marshal(item.Value)
	if err != nil {
		return err
	}
	if _, err := c.Client.Set(context.TODO(), key, body, item.Duration).Result(); err != nil {
		return err
	}

	return json.Unmarshal(body, result)
}

func (c *Redis) Remove(key string) {
	c.Client.Del(context.TODO(), key)
}
