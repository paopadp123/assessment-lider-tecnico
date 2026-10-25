package httpapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
)

const minSecretBytes = 32

// SignatureVerifier valida la firma HMAC-SHA256 de los webhooks del PSP.
//
// Supuesto documentado: el contrato no define qué se firma; se firma el texto
// "paymentId|status|occurredAt" con los valores tal como llegan, y la firma viaja en hexadecimal.
type SignatureVerifier struct {
	secret []byte
}

// NewSignatureVerifier exige un secreto de al menos 32 bytes (el tamaño de la salida de SHA-256).
func NewSignatureVerifier(secret string) (*SignatureVerifier, error) {
	if len(secret) < minSecretBytes {
		return nil, errors.New("webhook secret must have at least 32 bytes")
	}
	return &SignatureVerifier{secret: []byte(secret)}, nil
}

// Sign calcula la firma esperada. Lo usan las pruebas y documenta el contrato para el simulador.
func (v *SignatureVerifier) Sign(paymentID, status, occurredAt string) string {
	return hex.EncodeToString(v.mac(paymentID, status, occurredAt))
}

// Verify compara en tiempo constante (hmac.Equal) para no filtrar, por diferencias de tiempo,
// cuántos bytes de una firma falsa coinciden.
func (v *SignatureVerifier) Verify(paymentID, status, occurredAt, signature string) bool {
	got, err := hex.DecodeString(signature)
	if err != nil {
		return false
	}
	return hmac.Equal(got, v.mac(paymentID, status, occurredAt))
}

func (v *SignatureVerifier) mac(paymentID, status, occurredAt string) []byte {
	m := hmac.New(sha256.New, v.secret)
	m.Write([]byte(paymentID + "|" + status + "|" + occurredAt))
	return m.Sum(nil)
}
