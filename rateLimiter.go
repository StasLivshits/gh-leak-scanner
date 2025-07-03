package main

import (
	"context"
	"github.com/google/go-github/v62/github"
	"log"
	"sync"
	"time"
)

type RateLimiter struct {
	client       *github.Client
	ctx          context.Context
	mu           sync.Mutex
	checkCounter int
	checkEvery   int
}

func NewRateLimiter(ctx context.Context, client *github.Client, checkEvery int) *RateLimiter {
	return &RateLimiter{
		client:     client,
		ctx:        ctx,
		checkEvery: checkEvery,
	}
}

func (rl *RateLimiter) Check() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.checkCounter++
	if rl.checkCounter < rl.checkEvery {
		return // skip check
	}
	rl.checkCounter = 0 // reset

	rate, _, err := rl.client.RateLimits(rl.ctx)
	if err != nil {
		log.Printf("Could not check rate limit: %v", err)
		return
	}

	core := rate.GetCore()
	if core.Remaining == 0 {
		reset := core.Reset.Time
		sleep := time.Until(reset) + 2*time.Second
		log.Printf("Rate limit exceeded. Sleeping until %s (%v)", reset.Format(time.RFC1123), sleep)
		time.Sleep(sleep)
	} else {
		log.Printf("Rate limit OK: %d remaining", core.Remaining)
	}
}
