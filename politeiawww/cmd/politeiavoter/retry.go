package main

import (
	"fmt"
	"time"

	"github.com/decred/politeia/util"
)

type retry struct {
	retries uint
}

func (c *ctx) retryPush(r *retry) {
	c.Lock()
	defer c.Unlock()
	c.retryQ.PushBack(r)
}

func (c *ctx) retryPop() *retry {
	c.Lock()
	defer c.Unlock()

	e := c.retryQ.Front()
	if e == nil {
		return nil
	}
	return c.retryQ.Remove(e)
}

func (c *ctx) retryLoop() {
	defer c.retryWG.Done()

	for {
		// random timeout between 0 and 119 seconds
		wait, err := util.Random(1)
		if err != nil {
			// This really shouldn't happen so just use 33 seconds
			wait = []byte{33}
		} else {
			wait[0] = wait[0] % 120
		}

		select {
		case <-c.c:
			break
		case <-time.After(time.Duration(wait[0]) * time.Second):
			fmt.Printf("tick after %v\n",
				time.Duration(wait[0])*time.Second)
		}

		e := retryPop()
		if e == nil {
			// Nothing to do
			continue
		}

		// Vote

		// If failure Push to back

		// If Success we are done
	}
}
