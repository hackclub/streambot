package main

import "github.com/go-redis/redis"

type Config struct {
	db *redis.Client
}

func NewConfig(redisURL string) (Config, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return Config{}, err
	}

	client := redis.NewClient(opts)

	return Config{
		db: client,
	}, nil
}

func (c *Config) UserActive(id string) bool {
	if err := c.db.Get(id).Err(); err != nil {
		if err == redis.Nil {
			return true
		}
	}

	return false
}

func (c *Config) EnableUser(id string) {
	c.db.Del(id)

}

func (c *Config) DisableUser(id string) {
	c.db.Set(id, true, 0)
}

func (c *Config) ChannelActive(id string) bool {
	if err := c.db.Get(id).Err(); err != nil {
		if err == redis.Nil {
			return true
		}
	}

	return false
}

func (c *Config) EnableChannel(id string) {
	c.db.Del(id)
}

func (c *Config) DisableChannel(id string) {
	c.db.Set(id, true, 0)
}
