package main

import (
	"encoding/json"
	"fmt"

	"github.com/go-redis/redis"
	"github.com/ipinfo/go-ipinfo/ipinfo"
)

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

func (c *Config) StoreIPInfo(info ipinfo.Info) error {
	encoded, err := json.Marshal(info)
	if err != nil {
		fmt.Println("failed to write IP info to redis:", err)
		return err
	}

	c.db.Set("ip/"+info.IP.String(), encoded, 0)

	return nil
}

func (c *Config) GetIPInfo(ip string) (info ipinfo.Info, present bool, err error) {
	ipinfoStr, err := c.db.Get("ip/" + ip).Result()
	if err == redis.Nil {
		return info, false, nil
	} else if err != nil {
		fmt.Println("error getting IP info from DB:", err)
		return info, present, err
	}

	if err := json.Unmarshal([]byte(ipinfoStr), &info); err != nil {
		fmt.Println("error unmarshaling IP info gotten from DB:", err)
		return info, present, err
	}

	return info, true, nil
}

func (c *Config) StoreUserIP(userId, ip string) {
	c.db.Set("userip/"+userId, ip, 0)
}

func (c *Config) GetUserIP(userId string) (ip string, present bool, err error) {
	ip, err = c.db.Get("userip/" + userId).Result()
	if err == redis.Nil {
		return ip, false, nil
	} else if err != nil {
		fmt.Println("error getting user IP from DB:", err)
		return ip, present, err
	}

	return ip, true, nil
}

func (c *Config) GetUserIPInfo(userId string) (info ipinfo.Info, present bool, err error) {
	var ip string

	ip, present, err = c.GetUserIP(userId)
	if err != nil {
		return info, present, err
	}

	if !present {
		return info, false, nil
	}

	return c.GetIPInfo(ip)
}
