package engine_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LanreAkintayo/hookops/internal/engine"
)

func TestSign_KnownRFC4231Vector(t *testing.T) {
	// RFC 4231 Test Case 2
	// Key:  "Jefe"
	// Data: "what do ya want for nothing?"
	// Digest: 5bdcc146bf60754e6a042426089575c75a003f089d2739839dec58b964ec3843
	key := "Jefe"
	data := []byte("what do ya want for nothing?")
	expectedSignature := "sha256=5bdcc146bf60754e6a042426089575c75a003f089d2739839dec58b964ec3843"

	sig := engine.Sign(data, key)
	assert.Equal(t, expectedSignature, sig)
	assert.True(t, engine.Verify(data, key, sig))
}

func TestSign_Determinism(t *testing.T) {
	payload := []byte(`{"event":"payment.succeeded","amount":25000,"currency":"USD"}`)
	secret := "whsec_b4c6e9a18f2d4e5a9c0b1e3f7a8d2c4e"

	firstSig := engine.Sign(payload, secret)
	require.NotEmpty(t, firstSig)
	assert.Contains(t, firstSig, "sha256=")

	for i := 0; i < 50; i++ {
		sig := engine.Sign(payload, secret)
		assert.Equal(t, firstSig, sig, "signing must be 100% deterministic across multiple calls")
	}
}

func TestVerify_TamperProofing(t *testing.T) {
	secret := "whsec_super_secret_key_123"
	payload := []byte(`{"order_id":"ord_1001","status":"paid"}`)

	validSig := engine.Sign(payload, secret)
	require.True(t, engine.Verify(payload, secret, validSig), "valid signature must verify successfully")

	t.Run("fails when payload is modified by a single character", func(t *testing.T) {
		tamperedPayload := []byte(`{"order_id":"ord_1001","status":"void"}`)
		assert.False(t, engine.Verify(tamperedPayload, secret, validSig))
	})

	t.Run("fails when payload has extra trailing space", func(t *testing.T) {
		tamperedPayload := []byte(`{"order_id":"ord_1001","status":"paid"} `)
		assert.False(t, engine.Verify(tamperedPayload, secret, validSig))
	})

	t.Run("fails when verified with a different secret", func(t *testing.T) {
		wrongSecret := "whsec_wrong_secret_456"
		assert.False(t, engine.Verify(payload, wrongSecret, validSig))
	})

	t.Run("fails when signature hash is tampered", func(t *testing.T) {
		// Flip the last character of the signature
		tamperedSig := validSig[:len(validSig)-1] + "0"
		if tamperedSig == validSig {
			tamperedSig = validSig[:len(validSig)-1] + "1"
		}
		assert.False(t, engine.Verify(payload, secret, tamperedSig))
	})
}

func TestSignAndVerify_EdgeCases(t *testing.T) {
	secret := "whsec_test_secret"

	t.Run("empty payload produces valid signature and verifies", func(t *testing.T) {
		emptyPayload := []byte{}
		sig := engine.Sign(emptyPayload, secret)
		assert.NotEmpty(t, sig)
		assert.True(t, engine.Verify(emptyPayload, secret, sig))
	})

	t.Run("empty secret returns empty string from Sign and false from Verify", func(t *testing.T) {
		assert.Empty(t, engine.Sign([]byte("data"), ""))
		assert.False(t, engine.Verify([]byte("data"), "", "sha256=abcdef"))
	})

	t.Run("whitespace-only secret returns empty string and false from Verify", func(t *testing.T) {
		assert.Empty(t, engine.Sign([]byte("data"), "   \t\n  "))
		assert.False(t, engine.Verify([]byte("data"), "   ", "sha256=abcdef"))
	})

	t.Run("empty signature fails verification", func(t *testing.T) {
		assert.False(t, engine.Verify([]byte("data"), secret, ""))
		assert.False(t, engine.Verify([]byte("data"), secret, "   "))
	})

	t.Run("signature without sha256= prefix fails verification", func(t *testing.T) {
		sig := engine.Sign([]byte("data"), secret)
		rawHex := sig[len(engine.SignaturePrefix):]
		assert.False(t, engine.Verify([]byte("data"), secret, rawHex))
	})

	t.Run("signature with wrong algorithm prefix fails verification", func(t *testing.T) {
		sig := engine.Sign([]byte("data"), secret)
		rawHex := sig[len(engine.SignaturePrefix):]
		assert.False(t, engine.Verify([]byte("data"), secret, "sha1="+rawHex))
		assert.False(t, engine.Verify([]byte("data"), secret, "md5="+rawHex))
	})
}

func BenchmarkSign(b *testing.B) {
	payload := []byte(`{"event":"payment.succeeded","order_id":"ord_1001","amount":25000,"currency":"USD"}`)
	secret := "whsec_b4c6e9a18f2d4e5a9c0b1e3f7a8d2c4e"

	
	for b.Loop() {
		_ = engine.Sign(payload, secret)
	}
}

func BenchmarkVerify(b *testing.B) {
	payload := []byte(`{"event":"payment.succeeded","order_id":"ord_1001","amount":25000,"currency":"USD"}`)
	secret := "whsec_b4c6e9a18f2d4e5a9c0b1e3f7a8d2c4e"
	sig := engine.Sign(payload, secret)

	
	for b.Loop() {
		_ = engine.Verify(payload, secret, sig)
	}
}
