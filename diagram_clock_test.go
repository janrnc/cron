package cron

import (
	"fmt"
	"testing"
	"time"
)

var start = time.Date(2025, time.January, 15, 18, 0, 0, 0, time.UTC)

func TestStartTime(t *testing.T) {
	clock := NewDiagramClock(start, "", map[string]func(){})
	n := clock.Now()
	if n.Compare(start) != 0 {
		t.Fail()
	}
	n = clock.Now()
	if n.Compare(start) != 0 {
		t.Fail()
	}
}

func TestOneJobExecution(t *testing.T) {
	a := func() {
		fmt.Println("a")
	}
	clock := NewDiagramClock(start, "a", map[string]func(){
		"a": a,
	})
	go func() {
		timer, _ := clock.Timer(time.Date(2025, time.January, 16, 18, 0, 0, 0, time.UTC))
		<-timer
		clock.GetJob("a")()
		clock.Timer(time.Date(2025, time.January, 17, 18, 0, 0, 0, time.UTC))
	}()
	clock.Verify()
}

func TestSkipThenJob(t *testing.T) {
	a := func() {
		fmt.Println("a")
	}
	clock := NewDiagramClock(start, "-a", map[string]func(){
		"a": a,
	})
	go func() {
		clock.Timer(time.Date(2025, time.January, 16, 18, 0, 0, 0, time.UTC))
		timer, _ := clock.Timer(time.Date(2025, time.January, 17, 18, 0, 0, 0, time.UTC))
		<-timer
		clock.GetJob("a")()
		clock.Timer(time.Date(2025, time.January, 18, 18, 0, 0, 0, time.UTC))
	}()
	clock.Verify()
}
