using System.Net;
using System.Text;
using System.Text.Json;

namespace Issuing.UnitTests.Support;

/// <summary>Una solicitud que la ACL le hizo al emisor legado.</summary>
public sealed record LegacyCall(string Path, string? CorrelationId, string Body);

/// <summary>
/// Emisor legado falso con respuestas programadas por ruta. Si no hay nada programado, la consulta responde
/// "no existe" y la emisión responde una póliza válida.
/// </summary>
public sealed class FakeLegacyIssuer : HttpMessageHandler
{
    private readonly Queue<Func<CancellationToken, Task<HttpResponseMessage>>> _issueResponses = new();
    private readonly Queue<Func<CancellationToken, Task<HttpResponseMessage>>> _lookupResponses = new();
    private readonly List<LegacyCall> _calls = [];

    public IReadOnlyList<LegacyCall> Calls
    {
        get
        {
            lock (_calls)
            {
                return [.. _calls];
            }
        }
    }

    public IEnumerable<LegacyCall> IssueCalls => Calls.Where(c => c.Path == "/issue");

    public IEnumerable<LegacyCall> LookupCalls => Calls.Where(c => c.Path == "/policies");

    public FakeLegacyIssuer OnIssue(Func<CancellationToken, Task<HttpResponseMessage>> response)
    {
        _issueResponses.Enqueue(response);
        return this;
    }

    public FakeLegacyIssuer OnLookup(Func<CancellationToken, Task<HttpResponseMessage>> response)
    {
        _lookupResponses.Enqueue(response);
        return this;
    }

    public static Func<CancellationToken, Task<HttpResponseMessage>> Json(object body) =>
        _ => Task.FromResult(new HttpResponseMessage(HttpStatusCode.OK)
        {
            Content = new StringContent(JsonSerializer.Serialize(body), Encoding.UTF8, "application/json"),
        });

    public static Func<CancellationToken, Task<HttpResponseMessage>> Issued(string externalRef, string policyNumber) =>
        Json(new { result = "OK", code = "00", message = "EMITIDA", data = new { policy = Policy(externalRef, policyNumber) } });

    public static Func<CancellationToken, Task<HttpResponseMessage>> Error(string code) =>
        Json(new { result = "ERROR", code, message = "MENSAJE DEL LEGADO CON PLACA ABC123" });

    public static Func<CancellationToken, Task<HttpResponseMessage>> Found(string externalRef, string policyNumber) =>
        Json(new { result = "OK", code = "00", policies = new[] { Policy(externalRef, policyNumber) } });

    public static Func<CancellationToken, Task<HttpResponseMessage>> NotFound() =>
        Json(new { result = "OK", code = "00", policies = Array.Empty<object>() });

    /// <summary>JSON cortado: el formato inválido del simulador.</summary>
    public static Func<CancellationToken, Task<HttpResponseMessage>> Broken() =>
        _ => Task.FromResult(new HttpResponseMessage(HttpStatusCode.OK)
        {
            Content = new StringContent("{\"result\":\"OK\",\"data\":{\"pol", Encoding.UTF8, "application/json"),
        });

    public static Func<CancellationToken, Task<HttpResponseMessage>> Down() =>
        _ => Task.FromResult(new HttpResponseMessage(HttpStatusCode.ServiceUnavailable)
        {
            Content = new StringContent("<html><body>503</body></html>", Encoding.UTF8, "text/html"),
        });

    public static Func<CancellationToken, Task<HttpResponseMessage>> Raw(HttpStatusCode status, string mediaType, string body) =>
        _ => Task.FromResult(new HttpResponseMessage(status) { Content = new StringContent(body, Encoding.UTF8, mediaType) });

    /// <summary>No responde nunca: la ACL corta por su timeout.</summary>
    public static Func<CancellationToken, Task<HttpResponseMessage>> Hang() =>
        async ct =>
        {
            await Task.Delay(Timeout.Infinite, ct);
            throw new InvalidOperationException("unreachable");
        };

    protected override async Task<HttpResponseMessage> SendAsync(HttpRequestMessage request, CancellationToken cancellationToken)
    {
        string path = request.RequestUri?.AbsolutePath ?? string.Empty;
        string body = request.Content is null ? string.Empty : await request.Content.ReadAsStringAsync(cancellationToken);
        string? correlation = request.Headers.TryGetValues("X-Correlation-ID", out IEnumerable<string>? values) ? values.Single() : null;
        lock (_calls)
        {
            _calls.Add(new LegacyCall(path, correlation, body));
        }

        string externalRef = path == "/policies" ? request.RequestUri!.Query.Split('=')[1] : string.Empty;
        Func<CancellationToken, Task<HttpResponseMessage>> response = path switch
        {
            "/issue" => _issueResponses.TryDequeue(out Func<CancellationToken, Task<HttpResponseMessage>>? r) ? r : Issued(IssuedRef(body), "POL-2026-000001"),
            "/policies" => _lookupResponses.TryDequeue(out Func<CancellationToken, Task<HttpResponseMessage>>? r) ? r : NotFound(),
            _ => throw new InvalidOperationException($"unexpected legacy path {path} ({externalRef})"),
        };
        return await response(cancellationToken);
    }

    private static object Policy(string externalRef, string policyNumber) => new
    {
        policyNumber,
        number = policyNumber,
        statusCode = "A",
        status = "ACTIVA",
        externalRef,
        issueDate = "24/10/2026",
    };

    private static string IssuedRef(string body)
    {
        using JsonDocument doc = JsonDocument.Parse(body);
        return doc.RootElement.GetProperty("policyRequest").GetProperty("externalRef").GetString() ?? string.Empty;
    }
}
