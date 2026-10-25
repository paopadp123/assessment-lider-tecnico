using System.Net;
using System.Net.Http.Json;
using System.Text.Json;
using Issuing.Api.Domain;
using Microsoft.Extensions.Options;

namespace Issuing.Api.Legacy;

/// <summary>
/// Llama al emisor legado. Un solo intento por llamada: los reintentos, el circuit breaker y la reconciliación
/// están en el orquestador. Nunca lanza por fallas del emisor; las devuelve como resultado.
/// </summary>
public sealed class LegacyIssuerClient(HttpClient http, IOptions<LegacyIssuerOptions> options)
{
    public const string CorrelationHeader = "X-Correlation-ID";

    private readonly LegacyIssuerOptions _options = options.Value;

    public async Task<LegacyIssueResult> IssueAsync(LegacyIssueRequest request, string correlationId, CancellationToken ct)
    {
        ArgumentNullException.ThrowIfNull(request);
        using CancellationTokenSource timeout = CancellationTokenSource.CreateLinkedTokenSource(ct);
        timeout.CancelAfter(_options.IssueTimeout);
        using HttpRequestMessage message = new(HttpMethod.Post, new Uri("issue", UriKind.Relative))
        {
            Content = JsonContent.Create(request),
        };
        message.Headers.Add(CorrelationHeader, correlationId);

        try
        {
            using HttpResponseMessage response = await http.SendAsync(message, timeout.Token);
            if (response.StatusCode == HttpStatusCode.ServiceUnavailable)
            {
                return new LegacyIssueResult.Down("issuer answered 503");
            }

            int status = (int)response.StatusCode;
            if (status is >= 400 and < 500)
            {
                // El legado no aceptó la solicitud (URL, formato): no emitió y reintentar no lo arregla.
                return new LegacyIssueResult.Answered(new IssueOutcome.IntegrationFailed($"issuer answered {status}"));
            }

            if (response.StatusCode != HttpStatusCode.OK)
            {
                return new LegacyIssueResult.Unconfirmed($"issuer answered {status}");
            }

            LegacyIssueResponse? body = await response.Content.ReadFromJsonAsync<LegacyIssueResponse>(timeout.Token);
            return body is null
                ? new LegacyIssueResult.Unconfirmed("empty response")
                : LegacyMapper.FromIssueResponse(body, request.PolicyRequest.ExternalRef);
        }
        catch (OperationCanceledException) when (!ct.IsCancellationRequested)
        {
            // El emisor puede emitir y responder tarde ("timeout pero sí emitió").
            return new LegacyIssueResult.Unconfirmed($"no answer after {_options.IssueTimeout.TotalSeconds} s");
        }
        catch (Exception ex) when (ex is JsonException or NotSupportedException)
        {
            return new LegacyIssueResult.Unconfirmed("unreadable response");
        }
        catch (HttpRequestException ex)
        {
            // Una conexión cortada después de enviar la solicitud no prueba que no emitió.
            return new LegacyIssueResult.Unconfirmed($"connection error: {ex.HttpRequestError}");
        }
    }

    public async Task<LookupOutcome> FindAsync(string externalRef, string correlationId, CancellationToken ct)
    {
        using CancellationTokenSource timeout = CancellationTokenSource.CreateLinkedTokenSource(ct);
        timeout.CancelAfter(_options.LookupTimeout);
        Uri uri = new($"policies?externalRef={Uri.EscapeDataString(externalRef)}", UriKind.Relative);
        using HttpRequestMessage message = new(HttpMethod.Get, uri);
        message.Headers.Add(CorrelationHeader, correlationId);

        try
        {
            using HttpResponseMessage response = await http.SendAsync(message, timeout.Token);
            if (response.StatusCode != HttpStatusCode.OK)
            {
                return new LookupOutcome.Unavailable($"lookup answered {(int)response.StatusCode}");
            }

            LegacyLookupResponse? body = await response.Content.ReadFromJsonAsync<LegacyLookupResponse>(timeout.Token);
            return body is null
                ? new LookupOutcome.Unavailable("empty lookup response")
                : LegacyMapper.FromLookupResponse(body, externalRef);
        }
        catch (OperationCanceledException) when (!ct.IsCancellationRequested)
        {
            return new LookupOutcome.Unavailable($"no lookup answer after {_options.LookupTimeout.TotalSeconds} s");
        }
        catch (Exception ex) when (ex is JsonException or NotSupportedException)
        {
            return new LookupOutcome.Unavailable("unreadable lookup response");
        }
        catch (HttpRequestException ex)
        {
            return new LookupOutcome.Unavailable($"lookup connection error: {ex.HttpRequestError}");
        }
    }
}
