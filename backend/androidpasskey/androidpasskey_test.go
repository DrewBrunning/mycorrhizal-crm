package androidpasskey

import (
	"strings"
	"testing"
)

// Independently computed with python3 (hashlib/base64) and xxd|base64, not by
// this package: bytes 0x00..0x1f, and sha256("mycorrhizal-test-cert").
const (
	seqHex       = "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"
	seqColon     = "00:01:02:03:04:05:06:07:08:09:0A:0B:0C:0D:0E:0F:10:11:12:13:14:15:16:17:18:19:1A:1B:1C:1D:1E:1F"
	seqOrigin    = "android:apk-key-hash:AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8"
	certHex      = "a365e6d62835037a56fddcc88880f55fec99f4fd021ba4c0af509172342e6ec0"
	certColon    = "A3:65:E6:D6:28:35:03:7A:56:FD:DC:C8:88:80:F5:5F:EC:99:F4:FD:02:1B:A4:C0:AF:50:91:72:34:2E:6E:C0"
	certOrigin   = "android:apk-key-hash:o2Xm1ig1A3pW_dzIiID1X-yZ9P0CG6TAr1CRcjQubsA"
	publicOrigin = "https://crm.example.com"
)

func TestFingerprintGoldenVectors(t *testing.T) {
	for _, tc := range []struct{ in, colon, origin string }{
		{seqHex, seqColon, seqOrigin},
		{certHex, certColon, certOrigin},
	} {
		f, err := ParseFingerprint(tc.in)
		if err != nil {
			t.Fatal(err)
		}
		if f.Colon() != tc.colon {
			t.Errorf("Colon = %s, want %s", f.Colon(), tc.colon)
		}
		if f.Origin() != tc.origin {
			t.Errorf("Origin = %s, want %s", f.Origin(), tc.origin)
		}
	}
}

func TestParseFingerprintFormats(t *testing.T) {
	want, _ := ParseFingerprint(certHex)
	for name, in := range map[string]string{
		"plain lower":  certHex,
		"plain upper":  strings.ToUpper(certHex),
		"colon upper":  certColon,
		"colon lower":  strings.ToLower(certColon),
		"padded space": "  " + certColon + " ",
	} {
		got, err := ParseFingerprint(in)
		if err != nil || got != want {
			t.Errorf("%s: got %v err %v", name, got, err)
		}
	}
	for name, in := range map[string]string{
		"empty":        "",
		"short":        certHex[:62],
		"long":         certHex + "00",
		"non-hex":      strings.Repeat("zz", 32),
		"31 groups":    strings.TrimSuffix(certColon, ":C0"),
		"33 groups":    certColon + ":00",
		"3-digit grp":  "A3:65:E6D:" + certColon[9:],
		"colon+1 byte": "A3:" + certHex[2:],
	} {
		if _, err := ParseFingerprint(in); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestParseFingerprintsDedupeSortSkipBlank(t *testing.T) {
	got, err := ParseFingerprints([]string{certColon, "", seqHex, strings.ToLower(certHex), "  "})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Colon() != seqColon || got[1].Colon() != certColon {
		t.Fatalf("got %v", got)
	}
	if _, err := ParseFingerprints([]string{seqHex, "nope"}); err == nil {
		t.Fatal("malformed entry must fail the list")
	}
}

func TestValidatePublicHTTPSOrigin(t *testing.T) {
	ok := []string{
		publicOrigin, "https://crm.example.com/", "https://CRM.Example.COM", "https://crm.example.com:443",
		"https://a.b.example.org", "https://crm.example.com.", "https://my.home.example.net",
	}
	for _, u := range ok {
		if err := ValidatePublicHTTPSOrigin(u); err != nil {
			t.Errorf("%s should be accepted: %v", u, err)
		}
	}
	bad := map[string]string{
		"":                             "concrete origin",
		"*":                            "concrete origin",
		"://":                          "valid URL",
		"crm.example.com":              "valid URL",
		"http://crm.example.com":       "https",
		"https://192.168.1.20":         "IP address",
		"https://8.8.8.8":              "IP address",
		"https://[::1]":                "IP address",
		"https://[2001:db8::1]:443":    "IP address",
		"https://localhost":            "localhost",
		"https://crm":                  "single-label",
		"https://crm.local":            ".local",
		"https://crm.localhost":        ".localhost",
		"https://crm.lan":              ".lan",
		"https://crm.internal":         ".internal",
		"https://crm.home.arpa":        ".home.arpa",
		"https://crm.test":             ".test",
		"https://crm.example":          ".example",
		"https://crm.invalid":          ".invalid",
		"https://crm.example.com:8443": "port 8443",
		"https://a..example.com":       "empty label",
	}
	for u, wantReason := range bad {
		err := ValidatePublicHTTPSOrigin(u)
		if err == nil || !strings.Contains(err.Error(), wantReason) {
			t.Errorf("%q: err %v, want substring %q", u, err, wantReason)
		}
	}
}

func TestResolveMatrix(t *testing.T) {
	cases := []struct {
		name      string
		url       string
		enabled   bool
		operator  []string
		effective bool
		reason    string
	}{
		{"all good", publicOrigin, true, []string{certColon}, true, ""},
		{"switch off", publicOrigin, false, []string{certColon}, false, "WEBAUTHN_ANDROID_ENABLED is not set"},
		{"http", "http://crm.example.com", true, []string{certColon}, false, "https"},
		{"ip", "https://10.0.0.5", true, []string{certColon}, false, "IP address"},
		{"star", "*", true, []string{certColon}, false, "concrete origin"},
		{"empty", "", true, []string{certColon}, false, "concrete origin"},
		{"localhost", "https://localhost", true, []string{certColon}, false, "localhost"},
		{"lan suffix", "https://crm.lan", true, []string{certColon}, false, ".lan"},
		{"malformed fp", publicOrigin, true, []string{"zzz"}, false, "WEBAUTHN_ANDROID_CERT_SHA256"},
	}
	for _, tc := range cases {
		st := Resolve(tc.url, tc.enabled, tc.operator)
		if st.Effective != tc.effective {
			t.Errorf("%s: Effective = %v (reason %q)", tc.name, st.Effective, st.Reason)
		}
		if !tc.effective && !strings.Contains(st.Reason, tc.reason) {
			t.Errorf("%s: reason %q, want substring %q", tc.name, st.Reason, tc.reason)
		}
		if !tc.effective && (st.Origins() != nil || st.AssetLinksJSON() != nil) {
			t.Errorf("%s: a non-effective state must expose no origins and no assetlinks body", tc.name)
		}
	}
}

const (
	obtainiumColon  = "24:CF:16:6F:59:36:A6:AD:05:B4:4B:6B:56:9D:71:17:7A:03:8D:4A:06:0F:A4:85:E1:50:2E:73:29:EF:06:5E"
	obtainiumOrigin = "android:apk-key-hash:JM8Wb1k2pq0FtEtrVp1xF3oDjUoGD6SF4VAucynvBl4" // python3 urlsafe_b64encode of the raw digest
)

func TestBuiltinChannelsAreWellFormed(t *testing.T) {
	names := map[string]bool{}
	for _, c := range channels {
		names[c.name] = true
		if c.fingerprint != "" {
			if _, err := ParseFingerprint(c.fingerprint); err != nil {
				t.Errorf("channel %s: %v", c.name, err)
			}
		}
	}
	for _, n := range []string{"obtainium", "play", "foss"} {
		if !names[n] {
			t.Errorf("channel %s missing from the registry", n)
		}
	}
	// obtainium is verified (issue #1334); play and foss are pending and must
	// stay empty until real values are supplied.
	got := Builtin()
	if len(got) != 1 || got[0].Colon() != obtainiumColon || got[0].Origin() != obtainiumOrigin {
		t.Fatalf("Builtin() = %v", got)
	}
}

func TestResolveWithBuiltinOnlyNeedsSwitchAndDomain(t *testing.T) {
	st := Resolve(publicOrigin, true, nil)
	if !st.Effective || len(st.Fingerprints) != 1 {
		t.Fatalf("state = %+v", st)
	}
	// Empty registry (injected) and no operator fingerprints: off.
	off := resolve(nil, publicOrigin, true, nil)
	if off.Effective || !strings.Contains(off.Reason, "no signing-certificate fingerprints") {
		t.Fatalf("empty registry: %+v", off)
	}
}

func TestResolveMergesBuiltinAndOperatorDeduped(t *testing.T) {
	seq, _ := ParseFingerprint(seqHex)
	st := resolve([]Fingerprint{seq}, publicOrigin, true, []string{certHex, seqColon})
	if !st.Effective || len(st.Fingerprints) != 2 {
		t.Fatalf("state = %+v", st)
	}
	// built-in alone is enough (operator list optional)
	if st := resolve([]Fingerprint{seq}, publicOrigin, true, nil); !st.Effective || len(st.Fingerprints) != 1 {
		t.Fatalf("built-in only: %+v", st)
	}
	got := st.Origins()
	want := []string{seqOrigin, certOrigin}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("Origins = %v, want %v", got, want)
	}
}

func TestAssetLinksJSONGolden(t *testing.T) {
	st := Resolve(publicOrigin, true, []string{certColon, seqHex})
	want := `[{"relation":["delegate_permission/common.get_login_creds"],"target":{"namespace":"android_app","package_name":"com.mycorrhizal.crm","sha256_cert_fingerprints":["` +
		seqColon + `","` + obtainiumColon + `","` + certColon + `"]}}]`
	if got := string(st.AssetLinksJSON()); got != want {
		t.Fatalf("assetlinks =\n%s\nwant\n%s", got, want)
	}
}
