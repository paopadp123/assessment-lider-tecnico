// Package issuer implementa el puerto app.PolicyIssuer contra la capa de emisión (ACL en C#).
package issuer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/app"
	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/order"
	"github.com/paopadp123/assessment-lider-tecnico/orchestrator/internal/trace"
)

const maxResponseBytes = 64 << 10

// Client llama a la ACL. Los timeouts los controla el contexto de cada intento.
//
// Contrato limpio que la ACL (HU-07) expone hacia adentro:
//
//	POST /policies                   201 o 200 {policyNumber}   (200 = ya estaba emitida: idempotente por externalRef)
//	                                 422 {code, message}        rechazo de negocio definitivo
//	GET  /policies?externalRef={id}  200 {policyNumber} | 404
//
// Solo el 422 es definitivo. Cualquier otro status, los errores de red y las respuestas inválidas son transitorios.
type Client struct {
	baseURL string
	http    *http.Client
}

var _ app.PolicyIssuer = (*Client)(nil)

// NewClient crea el cliente de la capa de emisión.
func NewClient(baseURL string, httpClient *http.Client) *Client {
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), http: httpClient}
}

type issueRequest struct {
	ExternalRef string      `json:"externalRef"`
	QuoteID     string      `json:"quoteId"`
	Premium     json.Number `json:"premium"`
	Currency    string      `json:"currency"`
	Plate       string      `json:"plate"`
	DocumentID  string      `json:"documentId"`
	VehicleYear int         `json:"vehicleYear"`
	CityCode    string      `json:"cityCode"`
}

type policyResponse struct {
	PolicyNumber string `json:"policyNumber"`
}

type rejection struct {
	Code string `json:"code"`
}

// Issue emite la póliza.
func (c *Client) Issue(ctx context.Context, req app.IssueRequest) (order.PolicyNumber, error) {
	body, err := json.Marshal(issueRequest{
		ExternalRef: string(req.ExternalRef),
		QuoteID:     req.QuoteID,
		Premium:     json.Number(req.Premium),
		Currency:    req.Currency,
		Plate:       req.Plate.Raw(),
		DocumentID:  req.DocumentID.Raw(),
		VehicleYear: req.VehicleYear,
		CityCode:    req.CityCode,
	})
	if err != nil {
		return "", fmt.Errorf("encode issue request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/policies", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build issue request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if id := trace.ID(ctx); id != "" {
		httpReq.Header.Set(trace.Header, id) // el tercero puede cruzar sus logs con los de la compra
	}

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("%w: %w", app.ErrIssuerUnavailable, err)
	}
	defer drain(resp)

	switch {
	case resp.StatusCode == http.StatusUnprocessableEntity:
		// Solo el código: el mensaje de la ACL puede repetir datos personales y terminaría en los logs.
		var rej rejection
		_ = json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&rej)
		return "", fmt.Errorf("%w: code %q", app.ErrIssueRejected, rej.Code)
	case resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK:
		// Cualquier otro status (5xx, 429, y también 400/401/404) no es un rechazo de negocio: un error de
		// configuración o del request no debe dejar una orden pagada en REFUND_REQUIRED. Se reintenta y,
		// si persiste, la DLQ y la reconciliación lo hacen visible.
		return "", fmt.Errorf("%w: unexpected status %d", app.ErrIssuerUnavailable, resp.StatusCode)
	}
	return decodePolicy(resp)
}

// FindByExternalRef consulta si la póliza ya se emitió.
func (c *Client) FindByExternalRef(ctx context.Context, ref order.OrderID) (order.PolicyNumber, error) {
	u := c.baseURL + "/policies?externalRef=" + url.QueryEscape(string(ref))
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", fmt.Errorf("build policy lookup: %w", err)
	}
	if id := trace.ID(ctx); id != "" {
		httpReq.Header.Set(trace.Header, id) // el tercero puede cruzar sus logs con los de la compra
	}

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("%w: %w", app.ErrIssuerUnavailable, err)
	}
	defer drain(resp)

	switch resp.StatusCode {
	case http.StatusOK:
		return decodePolicy(resp)
	case http.StatusNotFound:
		return "", app.ErrPolicyNotFound
	default:
		return "", fmt.Errorf("%w: lookup status %d", app.ErrIssuerUnavailable, resp.StatusCode)
	}
}

// decodePolicy lee el número de póliza. Una respuesta exitosa sin número no es confiable: se trata como
// transitoria y el reintento consulta por externalRef antes de reemitir.
func decodePolicy(resp *http.Response) (order.PolicyNumber, error) {
	var out policyResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&out); err != nil {
		return "", fmt.Errorf("%w: invalid response body: %w", app.ErrIssuerUnavailable, err)
	}
	if out.PolicyNumber == "" {
		return "", fmt.Errorf("%w: response without policy number", app.ErrIssuerUnavailable)
	}
	return order.PolicyNumber(out.PolicyNumber), nil
}

// drain permite que net/http reutilice la conexión keep-alive.
func drain(resp *http.Response) {
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBytes))
	_ = resp.Body.Close()
}
