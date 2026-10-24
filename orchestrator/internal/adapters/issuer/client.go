// Package issuer implementa el puerto app.PolicyIssuer contra la capa de emisión (ACL en C#).
package issuer

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/app"
	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/order"
)

// Client llama a la ACL. Los timeouts los controla el contexto de cada intento.
type Client struct {
	baseURL string
	http    *http.Client
}

var _ app.PolicyIssuer = (*Client)(nil)

// NewClient crea el cliente de la capa de emisión.
func NewClient(baseURL string, httpClient *http.Client) *Client {
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), http: httpClient}
}

// Issue emite la póliza.
func (c *Client) Issue(ctx context.Context, req app.IssueRequest) (order.PolicyNumber, error) {
	return "", errors.New("not implemented")
}

// FindByExternalRef consulta si la póliza ya se emitió.
func (c *Client) FindByExternalRef(ctx context.Context, ref order.OrderID) (order.PolicyNumber, error) {
	return "", errors.New("not implemented")
}
