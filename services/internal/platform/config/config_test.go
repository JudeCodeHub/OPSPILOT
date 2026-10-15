package config

import (
	"errors"
	"log/slog"
	"testing"
	"time"
)

// errFatal is panicked by the test replacement of fatal.
var errFatal = errors.New("fatal called")

// expectFatal replaces fatal for the duration of the test and reports whether it was called.
func expectFatal(t *testing.T, fn func()) (called bool) {
	t.Helper()
	orig := fatal
	t.Cleanup(func() { fatal = orig })
	fatal = func(string, error) { panic(errFatal) }
	defer func() {
		if r := recover(); r != nil {
			if r != errFatal {
				panic(r)
			}
			called = true
		}
	}()
	fn()
	return false
}

func TestDefaultsWhenUnset(t *testing.T) {
	t.Setenv("X_STR", "")
	if got := String("X_STR", "def"); got != "def" {
		t.Errorf("String = %q, want def", got)
	}
	if got := Int("X_UNSET_INT", 7); got != 7 {
		t.Errorf("Int = %d, want 7", got)
	}
	if got := Float("X_UNSET_FLOAT", 0.5); got != 0.5 {
		t.Errorf("Float = %v, want 0.5", got)
	}
	if got := Bool("X_UNSET_BOOL", true); !got {
		t.Error("Bool = false, want true")
	}
	if got := Duration("X_UNSET_DUR", 3*time.Second); got != 3*time.Second {
		t.Errorf("Duration = %v, want 3s", got)
	}
}

func TestParsesValues(t *testing.T) {
	t.Setenv("X_STR", "  hello  ")
	t.Setenv("X_INT", "42")
	t.Setenv("X_FLOAT", "0.25")
	t.Setenv("X_BOOL", "true")
	t.Setenv("X_DUR", "250ms")

	if got := String("X_STR", ""); got != "hello" {
		t.Errorf("String = %q, want hello (trimmed)", got)
	}
	if got := Int("X_INT", 0); got != 42 {
		t.Errorf("Int = %d, want 42", got)
	}
	if got := Float("X_FLOAT", 0); got != 0.25 {
		t.Errorf("Float = %v, want 0.25", got)
	}
	if got := Bool("X_BOOL", false); !got {
		t.Error("Bool = false, want true")
	}
	if got := Duration("X_DUR", 0); got != 250*time.Millisecond {
		t.Errorf("Duration = %v, want 250ms", got)
	}
}

func TestInvalidValuesAreFatal(t *testing.T) {
	cases := map[string]func(){
		"int":      func() { Int("X_BAD", 0) },
		"float":    func() { Float("X_BAD", 0) },
		"bool":     func() { Bool("X_BAD", false) },
		"duration": func() { Duration("X_BAD", 0) },
	}
	for name, fn := range cases {
		t.Run(name, func(t *testing.T) {
			t.Setenv("X_BAD", "abc")
			if !expectFatal(t, fn) {
				t.Error("expected fatal for invalid value")
			}
		})
	}
}

func TestRequired(t *testing.T) {
	t.Setenv("X_REQ", "value")
	if got := Required("X_REQ"); got != "value" {
		t.Errorf("Required = %q", got)
	}
	t.Setenv("X_REQ", "")
	if !expectFatal(t, func() { Required("X_REQ") }) {
		t.Error("expected fatal for missing required value")
	}
}

func TestLoadBase(t *testing.T) {
	t.Setenv("SERVICE_NAME", "")
	t.Setenv("PORT", "9090")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("SHUTDOWN_TIMEOUT", "")

	b := LoadBase("svc")
	if b.ServiceName != "svc" || b.Port != 9090 || b.LogLevel != slog.LevelDebug || b.ShutdownTimeout != 10*time.Second {
		t.Errorf("unexpected base config: %+v", b)
	}
	if b.Addr() != ":9090" {
		t.Errorf("Addr = %q", b.Addr())
	}

	t.Setenv("LOG_LEVEL", "loud")
	if !expectFatal(t, func() { LoadBase("svc") }) {
		t.Error("expected fatal for invalid LOG_LEVEL")
	}
}
