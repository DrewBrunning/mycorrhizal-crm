package releaseworkflow

import (
	"fmt"
	"strings"
)

// Issue #2244: the versioned docs site publishes a release's docs as its root.
// Two quiet failure modes make a release never reach the site:
//
//   - a `push: tags` trigger on docs.yml: the github-pages environment deploys
//     only from main, so a tag run builds and is then refused, and
//     promote-rc.yml pushes the final tag with GITHUB_TOKEN, which starts no
//     workflow at all;
//   - a release path that never asks for a rebuild, so the site keeps the
//     previous release as "latest" until an unrelated docs merge.
//
// The working shape is: docs.yml runs on main (push or workflow_dispatch), and
// each final-release path dispatches it on main after its tag push, from a job
// that holds `actions: write` and only runs when a tag was really pushed.

// DocsPublishFiles holds the workflow texts CheckDocsPublish reads.
type DocsPublishFiles struct {
	Docs    string // .github/workflows/docs.yml
	Release string // .github/workflows/release.yml
	Promote string // .github/workflows/promote-rc.yml
}

const docsDispatch = "gh workflow run docs.yml"

// CheckDocsPublish returns one violation per broken link in that chain.
func CheckDocsPublish(f DocsPublishFiles) []string {
	var out []string
	docs, errs := parseDoc("docs.yml", f.Docs)
	out = append(out, errs...)
	if docs != nil {
		out = append(out, checkDocsTriggers(docs)...)
	}
	rel, errs := parseDoc("release.yml", f.Release)
	out = append(out, errs...)
	if rel != nil {
		out = append(out, checkDocsDispatch("release.yml", rel, "release", true)...)
	}
	pro, errs := parseDoc("promote-rc.yml", f.Promote)
	out = append(out, errs...)
	if pro != nil {
		out = append(out, checkDocsDispatch("promote-rc.yml", pro, "promote", false)...)
	}
	return out
}

func checkDocsTriggers(doc anyMap) []string {
	var out []string
	on := asMap(doc["on"]) // yaml.v3 keeps the bare key `on` a string
	if push := asMap(on["push"]); push != nil && (push["tags"] != nil || push["tags-ignore"] != nil) {
		out = append(out, "docs.yml: remove the push tag trigger -- the github-pages environment deploys only from main, so a tag run is refused; the release workflows dispatch docs.yml on main instead")
	}
	if _, ok := on["workflow_dispatch"]; !ok {
		out = append(out, "docs.yml: needs a workflow_dispatch trigger so the release workflows can publish a new release's docs")
	}
	return out
}

func checkDocsDispatch(file string, doc anyMap, tagJob string, gateDryRun bool) []string {
	var name string
	var job anyMap
	for n, j := range asMap(doc["jobs"]) {
		if strings.Contains(runBlob(asMap(j)), docsDispatch) {
			name, job = n, asMap(j)
			break
		}
	}
	if job == nil {
		return []string{fmt.Sprintf("%s: no job runs %q, so a release made here never reaches the docs site", file, docsDispatch)}
	}
	var out []string
	if !strings.Contains(runBlob(job), docsDispatch+" --ref main") {
		out = append(out, fmt.Sprintf("%s#%s: dispatch docs.yml with --ref main (the github-pages environment deploys only from main)", file, name))
	}
	if asStr(asMap(job["permissions"])["actions"]) != "write" {
		out = append(out, fmt.Sprintf("%s#%s: needs permissions actions: write to dispatch docs.yml", file, name))
	}
	if !contains(needsOf(job), tagJob) {
		out = append(out, fmt.Sprintf("%s#%s: needs %q, the job that pushes the tag, or docs.yml runs before the tag exists", file, name, tagJob))
	}
	if gateDryRun && !strings.Contains(asStr(job["if"]), "!inputs.dry_run") {
		out = append(out, fmt.Sprintf("%s#%s: skip the dispatch on a dry_run (no tag is pushed)", file, name))
	}
	return out
}
