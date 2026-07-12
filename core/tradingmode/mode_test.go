package tradingmode

import (
	"errors"
	"testing"
)

func TestParse_DefaultAndValid(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   string
		want Mode
	}{
		{"", ModePaper},
		{"paper", ModePaper},
		{"PAPER", ModePaper},
		{" live ", ModeLive},
		{"LIVE", ModeLive},
	}
	for _, tc := range cases {
		got, err := Parse(tc.in)
		if err != nil {
			t.Fatalf("Parse(%q): unexpected err: %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("Parse(%q)=%q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestParse_Invalid(t *testing.T) {
	t.Parallel()

	if _, err := Parse("sim"); err == nil {
		t.Fatal("expected error for invalid mode")
	}
}

func TestResolve_DefaultIsPaper(t *testing.T) {
	t.Parallel()

	getenv := func(string) string { return "" }
	mode, err := Resolve("", getenv)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if mode != ModePaper {
		t.Fatalf("default mode=%q, want paper", mode)
	}
}

func TestResolve_LiveRejectedWithoutAllowEnv(t *testing.T) {
	t.Parallel()

	getenv := func(key string) string {
		if key == EnvTradingMode {
			return ""
		}
		return ""
	}
	_, err := Resolve("live", getenv)
	if !errors.Is(err, ErrLiveNotAllowed) {
		t.Fatalf("Resolve(live) err=%v, want ErrLiveNotAllowed", err)
	}

	getenv = func(key string) string {
		if key == EnvTradingMode {
			return "live"
		}
		return ""
	}
	_, err = Resolve("paper", getenv)
	if !errors.Is(err, ErrLiveNotAllowed) {
		t.Fatalf("Resolve via SEQ_TRADING_MODE=live err=%v, want ErrLiveNotAllowed", err)
	}
}

func TestResolve_LiveRequiresDualOptIn(t *testing.T) {
	t.Parallel()

	getenv := func(key string) string {
		switch key {
		case EnvAllowLive:
			return "1"
		default:
			return ""
		}
	}
	mode, err := Resolve("live", getenv)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if mode != ModeLive {
		t.Fatalf("mode=%q, want live", mode)
	}

	getenv = func(key string) string {
		switch key {
		case EnvTradingMode:
			return "live"
		case EnvAllowLive:
			return "yes"
		default:
			return ""
		}
	}
	mode, err = Resolve("paper", getenv)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if mode != ModeLive {
		t.Fatalf("mode=%q, want live", mode)
	}
}

func TestRequireLive_PaperBlocks(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)

	if err := RequireLive(); !errors.Is(err, ErrPaperMode) {
		t.Fatalf("RequireLive in paper: %v", err)
	}

	Set(ModeLive)
	if err := RequireLive(); err != nil {
		t.Fatalf("RequireLive in live: %v", err)
	}
}

func TestCurrent_DefaultsToPaper(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)

	if Current() != ModePaper {
		t.Fatalf("Current()=%q, want paper", Current())
	}
}
