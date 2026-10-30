using System.Net;
using System.Text;
using System.Text.Json;
using Issuing.UnitTests.Support;

namespace Issuing.UnitTests;

/// <summary>
/// Pruebas de contrato (contracts/). La ACL es proveedora del orquestador y consumidora del emisor legado del
/// simulador: si cualquiera de los dos lados cambia un campo, estas pruebas fallan aquí o en el otro servicio.
/// </summary>
public sealed class ContractTests
{
    private static readonly ContractFile WithOrchestrator = ContractFile.Load("orchestrator-issuing-service.json");
    private static readonly ContractFile WithSimulator = ContractFile.Load("issuing-service-simulator.json");

    public static TheoryData<string> OrchestratorInteractions() => [.. WithOrchestrator.Interactions.Select(i => i.Name)];

    public static TheoryData<string> SimulatorInteractions() => [.. WithSimulator.Interactions.Select(i => i.Name)];

    /// <summary>Lado proveedor: la ACL responde al orquestador lo que dice el contrato, en cada estado del legado.</summary>
    [Theory]
    [MemberData(nameof(OrchestratorInteractions))]
    public async Task Answers_the_orchestrator_as_the_contract_says(string name)
    {
        ContractInteraction it = WithOrchestrator.Named(name);
        using IssuingApp app = new();
        Arrange(app.Legacy, it.ProviderState);
        using HttpClient client = app.CreateClient();
        using HttpRequestMessage request = new(new HttpMethod(it.Method), it.Url);
        if (it.RequestBody is JsonElement body)
        {
            request.Content = new StringContent(body.GetRawText(), Encoding.UTF8, "application/json");
        }

        using HttpResponseMessage response = await client.SendAsync(request);

        Assert.Equal(it.Status, (int)response.StatusCode);
        if (it.ResponseBody is JsonElement expected)
        {
            Assert.Empty(ContractFile.Match(expected, await response.Content.ReadAsStringAsync(), it.Exact));
        }
    }

    /// <summary>
    /// Lado consumidor: el emisor legado falso responde lo que dice el contrato con el simulador. La ACL tiene que
    /// enviarle la solicitud del contrato y traducir cada respuesta a lo que espera (emitida, rechazo o transitorio).
    /// </summary>
    [Theory]
    [MemberData(nameof(SimulatorInteractions))]
    public async Task Consumes_the_legacy_issuer_as_the_contract_says(string name)
    {
        ContractInteraction it = WithSimulator.Named(name);
        ContractInteraction emptyLookup = WithSimulator.Interactions.Single(i => i.Path == "/policies" && i.ConsumerExpects == "not-found");
        using IssuingApp app = new();
        using HttpClient client = app.CreateClient();
        HttpResponseMessage response;

        if (it.Path == "/issue")
        {
            // La ACL consulta antes de emitir y, ante un E-102, vuelve a consultar: las dos consultas no encuentran nada.
            app.Legacy.OnLookup(Respond(emptyLookup)).OnIssue(Respond(it)).OnLookup(Respond(emptyLookup));
            string orchestratorBody = WithOrchestrator.Named("issue a new policy").RequestBody!.Value.GetRawText();
            response = await client.PostAsync(new Uri("/policies", UriKind.Relative), new StringContent(orchestratorBody, Encoding.UTF8, "application/json"));

            LegacyCall sent = Assert.Single(app.Legacy.IssueCalls);
            Assert.Empty(ContractFile.MatchRequest(it, sent.Body));
        }
        else
        {
            app.Legacy.OnLookup(Respond(it));
            response = await client.GetAsync(new Uri(it.Url, UriKind.Relative));

            LegacyCall sent = Assert.Single(app.Legacy.LookupCalls);
            Assert.Equal(it.Path, sent.Path);
            Assert.Equal(it.Url[it.Path.Length..], sent.Query);
        }

        using (response)
        {
            HttpStatusCode expected = it.ConsumerExpects switch
            {
                "issued" => HttpStatusCode.Created,
                "rejected" => HttpStatusCode.UnprocessableEntity,
                "transient" => HttpStatusCode.ServiceUnavailable,
                "found" => HttpStatusCode.OK,
                "not-found" => HttpStatusCode.NotFound,
                _ => throw new InvalidOperationException($"consumerExpects sin cubrir: {it.ConsumerExpects}"),
            };
            Assert.Equal(expected, response.StatusCode);
            if (it.ConsumerExpects is "issued" or "found")
            {
                // La póliza que la ACL le entrega al orquestador es la que dijo el legado.
                Assert.Equal(LegacyPolicyNumber(it), await PolicyNumber(response));
            }
        }
    }

    private static string LegacyPolicyNumber(ContractInteraction it)
    {
        JsonElement body = it.ResponseBody!.Value;
        JsonElement policy = body.TryGetProperty("data", out JsonElement data) ? data.GetProperty("policy") : body.GetProperty("policies")[0];
        return policy.GetProperty("policyNumber").GetString()!;
    }

    private static async Task<string> PolicyNumber(HttpResponseMessage response)
    {
        using JsonDocument doc = JsonDocument.Parse(await response.Content.ReadAsStringAsync());
        return doc.RootElement.GetProperty("policyNumber").GetString()!;
    }

    /// <summary>Prepara el emisor legado falso en el estado que pide el contrato.</summary>
    private static void Arrange(FakeLegacyIssuer legacy, string state)
    {
        switch (state)
        {
            case "legacy-issues":
                legacy.OnLookup(FakeLegacyIssuer.NotFound()).OnIssue(FakeLegacyIssuer.Issued("order-1", "POL-2026-000007"));
                break;
            case "legacy-already-issued":
                legacy.OnLookup(FakeLegacyIssuer.Found("order-1", "POL-2026-000003"));
                break;
            case "legacy-rejects-invalid-plate":
                legacy.OnLookup(FakeLegacyIssuer.NotFound()).OnIssue(FakeLegacyIssuer.Error("E-104"));
                break;
            case "legacy-down":
                legacy.OnLookup(FakeLegacyIssuer.Down()).OnIssue(FakeLegacyIssuer.Down());
                break;
            case "legacy-has-no-policy":
                legacy.OnLookup(FakeLegacyIssuer.NotFound());
                break;
            default:
                throw new InvalidOperationException($"estado del proveedor sin preparar: {state}");
        }
    }

    private static Func<CancellationToken, Task<HttpResponseMessage>> Respond(ContractInteraction it) =>
        FakeLegacyIssuer.Raw((HttpStatusCode)it.Status, "application/json", it.ResponseBody?.GetRawText() ?? string.Empty);
}
