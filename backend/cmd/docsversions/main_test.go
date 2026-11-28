package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var tags = []string{
	"v1.5.0", "v1.6.0", "v1.7.0-rc.1", "v1.7.0-rc.2", "v1.7.0", "v1.7.1",
	"v1.8.0-rc.1", "not-a-tag", "v1.7.1-rc.1", "v2.0.0-rc.1",
}

func TestSelectLatestPatchPerMinorNewestFirst(t *testing.T) {
	got := Select(tags, "v1.6.0")
	require.Len(t, got, 2)
	assert.Equal(t, Version{Tag: "v1.7.1", Label: "1.7", Path: "", Latest: true}, got[0])
	assert.Equal(t, Version{Tag: "v1.6.0", Label: "1.6", Path: "1.6/"}, got[1])
}

func TestSelectSkipsReleaseCandidatesAndJunk(t *testing.T) {
	got := Select([]string{"v1.8.0-rc.1", "junk", "v1.7.0-rc.1"}, "v1.0.0")
	assert.Empty(t, got, "a release candidate is not published docs")
}

func TestSelectFloorExcludesOlder(t *testing.T) {
	got := Select(tags, "v1.7.0")
	require.Len(t, got, 1)
	assert.Equal(t, "v1.7.1", got[0].Tag)
}

func TestSelectLatestIsNewestEvenAcrossMajors(t *testing.T) {
	got := Select([]string{"v1.7.0", "v2.0.0", "v2.0.1"}, "v1.0.0")
	require.Len(t, got, 2)
	assert.True(t, got[0].Latest)
	assert.Equal(t, "v2.0.1", got[0].Tag)
	assert.Equal(t, "1.7/", got[1].Path)
}

func TestRunPrintsJSONWithNext(t *testing.T) {
	var out, errb bytes.Buffer
	code := run([]string{"-floor", "v1.7.0"}, strings.NewReader("v1.7.0\nv1.6.0\n"), &out, &errb)
	require.Equal(t, 0, code, errb.String())
	assert.JSONEq(t, `[
	  {"tag":"v1.7.0","label":"1.7","path":"","latest":true},
	  {"tag":"main","label":"next","path":"next/","latest":false,"unreleased":true}
	]`, out.String())
}

func TestRunWithNoReleasedTagsStillPublishesNextAtRoot(t *testing.T) {
	var out, errb bytes.Buffer
	require.Equal(t, 0, run([]string{"-floor", "v1.7.0"}, strings.NewReader(""), &out, &errb))
	assert.JSONEq(t, `[{"tag":"main","label":"next","path":"","latest":true,"unreleased":true}]`, out.String(),
		"before the first tag the unreleased docs are the whole site")
}

func TestRunRejectsBadFloor(t *testing.T) {
	var out, errb bytes.Buffer
	assert.Equal(t, 2, run([]string{"-floor", "banana"}, strings.NewReader(""), &out, &errb))
}

func TestRunRejectsUnknownFlag(t *testing.T) {
	var out, errb bytes.Buffer
	assert.Equal(t, 2, run([]string{"-nope"}, strings.NewReader(""), &out, &errb))
	assert.Empty(t, out.String(), "no version list on a usage error")
}
