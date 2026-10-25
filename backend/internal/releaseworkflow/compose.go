package releaseworkflow

import (
	"fmt"
	"strings"
)

// CanonicalComposeFile is the file an operator downloads. It must pull the
// published image rather than build, so the documented two-file install works
// without a checkout.
//
// BuildComposeFile is the source-build override. It is the only compose file
// that may declare `build:`, so CI can keep testing the checked-out tree while
// operators pull the signed release artifact.
const (
	CanonicalComposeFile = "docker-compose.yml"
	BuildComposeFile     = "docker-compose.build.yml"
)

const (
	composeService     = "mycorrhizal"
	publishedImageRepo = "ghcr.io/drewbrunning/mycorrhizal-crm"
)

// CheckComposeImages reports ways the operator/source-build split has drifted.
// canonical and build are the raw contents of CanonicalComposeFile and
// BuildComposeFile.
//
// The load-bearing properties:
//   - the canonical file must not contain `build:` anywhere on the service,
//     because Compose builds whenever a build section is present, silently
//     ignoring the `image:` an operator expected to pull;
//   - the canonical image must default IMAGE_TAG (so an existing .env without
//     the new variable keeps working) and point at this repo's registry;
//   - the build override must declare `build:` and tag a local image, never the
//     published tag, or a local build would overwrite the cached release image
//     a later `docker compose pull` relies on.
func CheckComposeImages(canonical, build string) []string {
	var findings []string

	canonicalDoc, errs := parseDoc(CanonicalComposeFile, canonical)
	if errs != nil {
		return errs
	}
	user := composeServiceOf(canonicalDoc)
	if user == nil {
		findings = append(findings, fmt.Sprintf("%s has no `%s` service", CanonicalComposeFile, composeService))
	} else {
		image := asStr(user["image"])
		if !strings.HasPrefix(image, publishedImageRepo+":") {
			findings = append(findings, fmt.Sprintf("%s `%s` service must pull the published image from `%s` (got %q)", CanonicalComposeFile, composeService, publishedImageRepo, image))
		}
		if !strings.Contains(image, "${IMAGE_TAG:-latest}") {
			findings = append(findings, fmt.Sprintf("%s `%s` service image must default `IMAGE_TAG` to `latest` (got %q) so an existing .env without IMAGE_TAG keeps working", CanonicalComposeFile, composeService, image))
		}
		if _, ok := user["build"]; ok {
			findings = append(findings, fmt.Sprintf("%s `%s` service must not declare `build:`: Compose builds whenever a build section is present, so `docker compose up -d` would silently ignore the published image", CanonicalComposeFile, composeService))
		}
	}

	buildDoc, errs := parseDoc(BuildComposeFile, build)
	if errs != nil {
		return append(findings, errs...)
	}
	source := composeServiceOf(buildDoc)
	if source == nil {
		findings = append(findings, fmt.Sprintf("%s has no `%s` service", BuildComposeFile, composeService))
	} else {
		if source["build"] == nil {
			findings = append(findings, fmt.Sprintf("%s `%s` service must declare `build:` so source builds and CI exercise the checked-out tree", BuildComposeFile, composeService))
		}
		image := asStr(source["image"])
		if image == "" {
			findings = append(findings, fmt.Sprintf("%s `%s` service must pin a local `image:` tag so `--build` has a stable name", BuildComposeFile, composeService))
		} else if strings.Contains(image, "ghcr.io") {
			findings = append(findings, fmt.Sprintf("%s `%s` service must not reuse the published image tag in `image:` (got %q): a local build would overwrite the cached release image", BuildComposeFile, composeService, image))
		}
	}

	return findings
}

func composeServiceOf(doc anyMap) anyMap {
	return asMap(asMap(doc["services"])[composeService])
}
