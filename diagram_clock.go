package cron

import (
	"sync"
	"time"
)

type DiagramClock struct {
	current    time.Time
	diagram    string
	jobs       map[string]func()
	wg         *sync.WaitGroup
	completion chan struct{}
	completed  bool
}

func NewDiagramClock(start time.Time, diagram string, funcs map[string]func()) *DiagramClock {
	wg := &sync.WaitGroup{}
	jobs := map[string]func(){}
	for key, fn := range funcs {
		jobs[key] = func() {
			defer wg.Done()
			fn()
		}
	}
	return &DiagramClock{
		current:    start,
		diagram:    diagram,
		jobs:       jobs,
		wg:         wg,
		completion: make(chan struct{}, 1),
	}
}

func (c *DiagramClock) Now() time.Time {
	return c.current
}

func (c *DiagramClock) Timer(t time.Time) (<-chan struct{}, func()) {
	if c.completed {
		return neverTimer()
	}
	c.wg.Wait()
	if len(c.diagram) == 0 {
		c.completion <- struct{}{}
		return neverTimer()
	}
	next := string(c.diagram[0])
	c.diagram = c.diagram[1:]
	if next == "-" {
		if len(c.diagram) == 0 {
			c.completion <- struct{}{}
		}
		return neverTimer()
	}
	c.current = t
	c.wg.Add(1)
	return immediateTimer()
}

func (c *DiagramClock) Verify() {
	<-c.completion
}

func (c *DiagramClock) GetJob(key string) func() {
	return c.jobs[key]
}
