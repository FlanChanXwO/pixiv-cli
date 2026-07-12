package staticlib

import "testing"

func TestIsDigestSourceAcceptsPlatformSeparators(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		rel         string
		includeLock bool
		want        bool
	}{
		{name: "crate source with slash", rel: "src/lib.rs", includeLock: true, want: true},
		{name: "crate source with backslash", rel: `src\lib.rs`, includeLock: true, want: true},
		{name: "nested crate source with backslash", rel: `src\encoder\gif.rs`, includeLock: true, want: true},
		{name: "Cargo config with backslash", rel: `.cargo\config.toml`, includeLock: true, want: true},
		{name: "vendored source with backslash", rel: `vendor\gif\src\lib.rs`, includeLock: true, want: true},
		{name: "quantette source with backslash", rel: `src\quantize\mod.rs`, includeLock: false, want: true},
		{name: "quantette Cargo config excluded", rel: `.cargo\config.toml`, includeLock: false, want: false},
		{name: "non-Rust source excluded", rel: `src\README.md`, includeLock: true, want: false},
		{name: "target artifact excluded", rel: `target\release\ugoira_rs.lib`, includeLock: true, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := isDigestSource(tt.rel, tt.includeLock); got != tt.want {
				t.Fatalf("isDigestSource(%q, %t) = %t, want %t", tt.rel, tt.includeLock, got, tt.want)
			}
		})
	}
}
