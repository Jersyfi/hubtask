// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// rfcSecret is RFC 6238 Appendix B's SHA-1 test secret: the ASCII digits 1..0 repeated.
var rfcSecret = []byte("12345678901234567890")

// The RFC's own vectors, truncated to the six digits every authenticator shows: the appendix
// lists eight-digit codes, and six digits are the last six of them by construction.
func TestTotpMatchesTheRFCVectors(t *testing.T) {
	cases := []struct {
		unix int64
		want string
	}{
		{59, "287082"},
		{1111111109, "081804"},
		{1111111111, "050471"},
		{1234567890, "005924"},
		{2000000000, "279037"},
		{20000000000, "353130"},
	}
	for _, c := range cases {
		step := TotpStep(time.Unix(c.unix, 0))
		if got := TotpCode(rfcSecret, step); got != c.want {
			t.Errorf("T=%d: code %s, want %s", c.unix, got, c.want)
		}
	}
}

func TestVerifyTotpAllowsOneStepOfDriftEitherSide(t *testing.T) {
	now := time.Unix(1111111111, 0)
	current := TotpStep(now)

	for _, delta := range []int64{-1, 0, 1} {
		code := TotpCode(rfcSecret, current+delta)
		if _, ok := VerifyTotp(rfcSecret, code, now, 0); !ok {
			t.Errorf("a code %+d steps out was refused", delta)
		}
	}
	for _, delta := range []int64{-2, 2} {
		code := TotpCode(rfcSecret, current+delta)
		if _, ok := VerifyTotp(rfcSecret, code, now, 0); ok {
			t.Errorf("a code %+d steps out verified", delta)
		}
	}
}

// H-02's replay refusal: the same step never verifies twice, and the drift window cannot be
// used to slide backwards past an accepted step.
func TestVerifyTotpRefusesReplay(t *testing.T) {
	now := time.Unix(1111111111, 0)
	code := TotpCode(rfcSecret, TotpStep(now))

	step, ok := VerifyTotp(rfcSecret, code, now, 0)
	if !ok {
		t.Fatal("the first presentation was refused")
	}
	if _, ok := VerifyTotp(rfcSecret, code, now, step); ok {
		t.Fatal("the same code verified twice")
	}
	// The previous window's code after the current one was accepted: refused, because accepting
	// it would be a replay one step removed.
	earlier := TotpCode(rfcSecret, TotpStep(now)-1)
	if _, ok := VerifyTotp(rfcSecret, earlier, now, step); ok {
		t.Fatal("an earlier step verified after a later one")
	}
}

func TestVerifyTotpRefusesTheWrong(t *testing.T) {
	now := time.Unix(1111111111, 0)
	for name, presented := range map[string]string{
		"a wrong code":     "000000",
		"an empty code":    "",
		"garbage":          "abcdef",
		"an 8-digit paste": "00" + TotpCode(rfcSecret, TotpStep(now)),
	} {
		if _, ok := VerifyTotp(rfcSecret, presented, now, 0); ok {
			t.Errorf("%s verified", name)
		}
	}
}

func TestTheProvisioningURICarriesWhatAuthenticatorsExpect(t *testing.T) {
	uri := TotpProvisioningURI("Hubtask", "bert@example.org", rfcSecret)
	for _, want := range []string{
		"otpauth://totp/Hubtask:bert@example.org?",
		"secret=" + TotpSecretBase32(rfcSecret),
		"issuer=Hubtask",
		"algorithm=SHA1",
		"digits=6",
		"period=30",
	} {
		if !strings.Contains(uri, want) {
			t.Errorf("uri %q lacks %q", uri, want)
		}
	}
	if strings.Contains(TotpSecretBase32(rfcSecret), "=") {
		t.Error("the base32 secret carries padding no authenticator expects")
	}
}

func TestRecoveryCodesFormatAndNormalise(t *testing.T) {
	material := bytes.Repeat([]byte{0x5A}, RecoveryCodeCount*RecoveryCodeBytes)
	codes, err := NewRecoveryCodes(material)
	if err != nil {
		t.Fatalf("minting: %v", err)
	}
	if len(codes) != RecoveryCodeCount {
		t.Fatalf("%d codes, want %d", len(codes), RecoveryCodeCount)
	}
	for _, code := range codes {
		if len(strings.Split(code, "-")) != 4 {
			t.Errorf("code %q is not grouped for reading aloud", code)
		}
		mangled := " " + strings.ToLower(strings.ReplaceAll(code, "-", " ")) + " "
		if NormalizeRecoveryCode(mangled) != NormalizeRecoveryCode(code) {
			t.Errorf("a mangled reading of %q does not normalise back", code)
		}
	}

	if _, err := NewRecoveryCodes(material[:5]); err == nil {
		t.Error("short material minted codes")
	}
}

func TestAPendingCredentialDiesByClockAndByUse(t *testing.T) {
	now := time.Unix(1111111111, 0)
	live := PendingCredential{CreatedAt: now, ExpiresAt: now.Add(PendingLifetime)}

	if err := live.Verify(now); err != nil {
		t.Errorf("a live credential refused: %v", err)
	}
	expired := live.Verify(now.Add(PendingLifetime + time.Second))
	consumed := PendingCredential{
		CreatedAt: now, ExpiresAt: now.Add(PendingLifetime), ConsumedAt: now,
	}.Verify(now)
	if expired == nil || consumed == nil {
		t.Fatal("a dead credential verified")
	}
	// One indistinguishable refusal: which of the two ended a stolen token is not for its
	// thief to learn.
	if expired.Error() != consumed.Error() {
		t.Errorf("%q and %q differ", expired, consumed)
	}
}

func TestAPendingTokenRoundTrips(t *testing.T) {
	minted, err := NewPendingToken(sessionTenant, sessionSecret())
	if err != nil {
		t.Fatalf("minting: %v", err)
	}
	if !strings.HasPrefix(minted.Secret(), PendingTokenPrefix) {
		t.Fatalf("minted %q, want the %q prefix", minted.Secret(), PendingTokenPrefix)
	}
	parsed, err := ParsePendingToken(minted.Secret())
	if err != nil || parsed.TenantID() != sessionTenant {
		t.Fatalf("parsing what was minted: %v, %v", parsed.TenantID(), err)
	}
	if _, err := ParsePendingToken(strings.Replace(minted.Secret(), PendingTokenPrefix, RefreshTokenPrefix, 1)); err == nil {
		t.Error("a refresh-prefixed token parsed as pending")
	}
}
