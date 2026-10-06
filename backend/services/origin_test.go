package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSameOrigin(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"https://gp.example", "https://gp.example/sub/path", true},
		{"https://gp.example", "https://GP.EXAMPLE:443", true},
		{"http://gp.example", "http://gp.example:80/x", true},
		{"http://gp.example", "https://gp.example", false},
		{"https://gp.example", "https://gp.example:8443", false},
		{"https://gp.example", "https://gp.example.evil.test", false},
		{"://bad", "https://gp.example", false},
		{"https://gp.example", "://bad", false},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, SameOrigin(tc.a, tc.b), "%s vs %s", tc.a, tc.b)
	}
}
