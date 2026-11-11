package main

import (
	"os"
	"testing"

	"mycorrhizal/internal/gittest"
)

// TestMain detaches the git fixtures from any ambient GIT_DIR (see gittest).
func TestMain(m *testing.M) { os.Exit(gittest.Main(m)) }
