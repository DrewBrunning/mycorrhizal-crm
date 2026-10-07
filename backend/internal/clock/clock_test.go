package clock

import (
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

var t0 = time.Date(2030, 6, 1, 12, 0, 0, 0, time.UTC)

func TestSystem_TracksWallClock(t *testing.T) {
	before := time.Now()
	got := System{}.Now()
	after := time.Now()
	if got.Before(before) || got.After(after) {
		t.Fatalf("System.Now() = %v, want within [%v, %v]", got, before, after)
	}
}

func TestFunc_AdaptsAFunction(t *testing.T) {
	var c Clock = Func(func() time.Time { return t0 })
	if !c.Now().Equal(t0) {
		t.Fatalf("Func clock = %v, want %v", c.Now(), t0)
	}
}

func TestFake_SetAdvanceAndRewind(t *testing.T) {
	f := NewFake(t0)
	if !f.Now().Equal(t0) {
		t.Fatalf("NewFake = %v", f.Now())
	}
	f.Advance(90 * time.Minute)
	if want := t0.Add(90 * time.Minute); !f.Now().Equal(want) {
		t.Fatalf("after Advance = %v, want %v", f.Now(), want)
	}
	f.Advance(-2 * time.Hour)
	if want := t0.Add(-30 * time.Minute); !f.Now().Equal(want) {
		t.Fatalf("after negative Advance = %v, want %v", f.Now(), want)
	}
	other := t0.AddDate(1, 0, 0)
	f.Set(other)
	if !f.Now().Equal(other) {
		t.Fatalf("after Set = %v, want %v", f.Now(), other)
	}
}

// A Fake does not move on its own — that is the whole point.
func TestFake_DoesNotTick(t *testing.T) {
	f := NewFake(t0)
	for i := 0; i < 1000; i++ {
		if !f.Now().Equal(t0) {
			t.Fatal("Fake advanced without Advance/Set")
		}
	}
}

func TestFake_ConcurrentUse(t *testing.T) {
	f := NewFake(t0)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); f.Advance(time.Second) }()
		go func() { defer wg.Done(); _ = f.Now() }()
	}
	wg.Wait()
	if want := t0.Add(50 * time.Second); !f.Now().Equal(want) {
		t.Fatalf("after 50 concurrent Advance(1s) = %v, want %v", f.Now(), want)
	}
}

func TestOr(t *testing.T) {
	if _, ok := Or(nil).(System); !ok {
		t.Fatal("Or(nil) must be System{}")
	}
	f := NewFake(t0)
	if Or(f) != Clock(f) {
		t.Fatal("Or(c) must return c unchanged")
	}
}

func TestContext_InstallAndFromContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)

	if _, ok := FromContext(c).(System); !ok {
		t.Fatal("no clock installed must yield System{}")
	}

	f := NewFake(t0)
	Install(c, f)
	if FromContext(c) != Clock(f) {
		t.Fatal("FromContext must return the installed clock")
	}

	// A wrong-typed value under the key falls back to the system clock.
	c.Set(ContextKey, "not a clock")
	if _, ok := FromContext(c).(System); !ok {
		t.Fatal("a non-Clock value must yield System{}")
	}

	// Install(nil) stores the system clock.
	Install(c, nil)
	if _, ok := FromContext(c).(System); !ok {
		t.Fatal("Install(nil) must store System{}")
	}
}

func TestInstallIfAbsent_NeverOverwrites(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)

	fake := NewFake(t0)
	InstallIfAbsent(c, fake)
	if FromContext(c) != Clock(fake) {
		t.Fatal("InstallIfAbsent must install when nothing is present")
	}
	InstallIfAbsent(c, System{})
	if FromContext(c) != Clock(fake) {
		t.Fatal("InstallIfAbsent must not overwrite an installed clock")
	}
}
