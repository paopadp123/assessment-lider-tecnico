using System.Diagnostics;
using System.Net;
using System.Net.Http.Json;
using System.Text.Json;
using Issuing.UnitTests.Support;

namespace Issuing.UnitTests;

/// <summary>La ACL completa contra un emisor legado falso: lo que ve el orquestador ante cada respuesta del legado.</summary>
public sealed class PolicyEndpointsTests
{
    private static readonly object ValidBody = new
    {
        externalRef = "order-1",
        quoteId = "qt_1",
        premium = 412000,
        currency = "COP",
        plate = "ABC123",
        documentId = "1020304050",
        vehicleYear = 2020,
        cityCode = "11001",
    };

    [Fact]
    public async Task New_policy_is_issued_with_201_and_the_correlation_id_reaches_the_legacy_issuer()
    {
        using IssuingApp app = new();
        app.Legacy.OnIssue(FakeLegacyIssuer.Issued("order-1", "POL-2026-000007"));

        using HttpResponseMessage response = await Issue(app, correlationId: "corr-compra-1");

        Assert.Equal(HttpStatusCode.Created, response.StatusCode);
        Assert.Equal("POL-2026-000007", await PolicyNumber(response));
        Assert.Equal("corr-compra-1", response.Headers.GetValues("X-Correlation-ID").Single());
        Assert.Equal(["/policies", "/issue"], app.Legacy.Calls.Select(c => c.Path));
        Assert.All(app.Legacy.Calls, c => Assert.Equal("corr-compra-1", c.CorrelationId));
        using JsonDocument sent = JsonDocument.Parse(app.Legacy.IssueCalls.Single().Body);
        Assert.Equal("order-1", sent.RootElement.GetProperty("policyRequest").GetProperty("externalReference").GetString());
    }

    [Fact]
    public async Task Already_issued_policy_is_returned_with_200_without_issuing_again()
    {
        using IssuingApp app = new();
        app.Legacy.OnLookup(FakeLegacyIssuer.Found("order-1", "POL-2026-000003"));

        using HttpResponseMessage response = await Issue(app);

        Assert.Equal(HttpStatusCode.OK, response.StatusCode);
        Assert.Equal("POL-2026-000003", await PolicyNumber(response));
        Assert.Empty(app.Legacy.IssueCalls);
    }

    [Fact]
    public async Task Business_error_hidden_in_a_200_becomes_422_with_a_clean_code()
    {
        using IssuingApp app = new();
        app.Legacy.OnIssue(FakeLegacyIssuer.Error("E-104"));

        using HttpResponseMessage response = await Issue(app);

        Assert.Equal(HttpStatusCode.UnprocessableEntity, response.StatusCode);
        string body = await response.Content.ReadAsStringAsync();
        Assert.Contains("\"code\":\"INVALID_PLATE\"", body, StringComparison.Ordinal);
        Assert.DoesNotContain("ABC123", body, StringComparison.Ordinal); // el mensaje del legado no se reenvía
    }

    [Theory]
    [InlineData("E-901", HttpStatusCode.ServiceUnavailable)]
    [InlineData("E-100", HttpStatusCode.BadGateway)]
    [InlineData("E-555", HttpStatusCode.BadGateway)]
    public async Task Technical_and_integration_errors_are_not_rejections(string legacyCode, HttpStatusCode expected)
    {
        using IssuingApp app = new();
        app.Legacy.OnIssue(FakeLegacyIssuer.Error(legacyCode));

        using HttpResponseMessage response = await Issue(app);

        Assert.Equal(expected, response.StatusCode);
    }

    [Fact]
    public async Task Active_policy_rejection_is_confirmed_with_a_lookup_before_answering_422()
    {
        using IssuingApp app = new();
        app.Legacy.OnIssue(FakeLegacyIssuer.Error("E-102"));

        using HttpResponseMessage response = await Issue(app);

        Assert.Equal(HttpStatusCode.UnprocessableEntity, response.StatusCode);
        Assert.Contains("\"code\":\"VEHICLE_HAS_ACTIVE_POLICY\"", await response.Content.ReadAsStringAsync(), StringComparison.Ordinal);
        Assert.Equal(["/policies", "/issue", "/policies"], app.Legacy.Calls.Select(c => c.Path));
    }

    [Fact]
    public async Task Active_policy_that_turns_out_to_be_ours_is_not_a_rejection()
    {
        // Un intento anterior emitió sin confirmarse; la consulta previa aún no lo veía.
        using IssuingApp app = new();
        app.Legacy
            .OnIssue(FakeLegacyIssuer.Error("E-102"))
            .OnLookup(FakeLegacyIssuer.NotFound())
            .OnLookup(FakeLegacyIssuer.Found("order-1", "POL-2026-000021"));

        using HttpResponseMessage response = await Issue(app);

        Assert.Equal(HttpStatusCode.OK, response.StatusCode);
        Assert.Equal("POL-2026-000021", await PolicyNumber(response));
    }

    [Fact]
    public async Task Client_error_from_the_legacy_issuer_is_an_integration_failure_without_lookup()
    {
        using IssuingApp app = new();
        app.Legacy.OnIssue(FakeLegacyIssuer.Raw(HttpStatusCode.BadRequest, "text/plain", "bad request"));

        using HttpResponseMessage response = await Issue(app);

        Assert.Equal(HttpStatusCode.BadGateway, response.StatusCode);
        Assert.Single(app.Legacy.LookupCalls);
    }

    [Fact]
    public async Task Html_instead_of_json_is_unreadable_and_confirmed_with_a_lookup()
    {
        using IssuingApp app = new();
        app.Legacy.OnIssue(FakeLegacyIssuer.Raw(HttpStatusCode.OK, "text/html", "<html>OK</html>"));

        using HttpResponseMessage response = await Issue(app);

        Assert.Equal(HttpStatusCode.ServiceUnavailable, response.StatusCode);
        Assert.Equal(2, app.Legacy.LookupCalls.Count());
    }

    [Fact]
    public async Task Timeout_but_issued_is_confirmed_with_a_lookup_and_answered_201()
    {
        using IssuingApp app = new();
        app.Legacy
            .OnIssue(FakeLegacyIssuer.Hang())
            .OnLookup(FakeLegacyIssuer.NotFound())
            .OnLookup(FakeLegacyIssuer.Found("order-1", "POL-2026-000011"));
        Stopwatch elapsed = Stopwatch.StartNew();

        using HttpResponseMessage response = await Issue(app);

        Assert.Equal(HttpStatusCode.Created, response.StatusCode);
        Assert.Equal("POL-2026-000011", await PolicyNumber(response));
        Assert.Equal(["/policies", "/issue", "/policies"], app.Legacy.Calls.Select(c => c.Path));
        Assert.True(elapsed.Elapsed < TimeSpan.FromSeconds(5), $"tardó {elapsed.Elapsed}");
    }

    [Fact]
    public async Task Unreadable_answer_without_policy_afterwards_is_503()
    {
        using IssuingApp app = new();
        app.Legacy.OnIssue(FakeLegacyIssuer.Broken());

        using HttpResponseMessage response = await Issue(app);

        Assert.Equal(HttpStatusCode.ServiceUnavailable, response.StatusCode);
        Assert.Equal(2, app.Legacy.LookupCalls.Count());
    }

    [Fact]
    public async Task Legacy_issuer_down_is_503()
    {
        using IssuingApp app = new();
        app.Legacy.OnIssue(FakeLegacyIssuer.Down());

        using HttpResponseMessage response = await Issue(app);

        Assert.Equal(HttpStatusCode.ServiceUnavailable, response.StatusCode);
    }

    [Fact]
    public async Task Without_a_working_lookup_nothing_is_issued()
    {
        using IssuingApp app = new();
        app.Legacy.OnLookup(FakeLegacyIssuer.Down());

        using HttpResponseMessage response = await Issue(app);

        Assert.Equal(HttpStatusCode.ServiceUnavailable, response.StatusCode);
        Assert.Empty(app.Legacy.IssueCalls);
    }

    [Fact]
    public async Task Invalid_request_is_400_with_the_fields_and_never_reaches_the_legacy_issuer()
    {
        using IssuingApp app = new();
        using HttpClient client = app.CreateClient();

        using HttpResponseMessage response = await client.PostAsJsonAsync(
            new Uri("/policies", UriKind.Relative),
            new { externalRef = "order-1", premium = -1, plate = "X" });

        Assert.Equal(HttpStatusCode.BadRequest, response.StatusCode);
        using JsonDocument problem = JsonDocument.Parse(await response.Content.ReadAsStringAsync());
        JsonElement errors = problem.RootElement.GetProperty("errors");
        Assert.True(errors.TryGetProperty("plate", out _));
        Assert.True(errors.TryGetProperty("premium", out _));
        Assert.Empty(app.Legacy.Calls);
    }

    [Fact]
    public async Task Lookup_answers_200_404_or_503()
    {
        using IssuingApp app = new();
        app.Legacy
            .OnLookup(FakeLegacyIssuer.Found("order-1", "POL-2026-000001"))
            .OnLookup(FakeLegacyIssuer.NotFound())
            .OnLookup(FakeLegacyIssuer.Down());
        using HttpClient client = app.CreateClient();
        Uri uri = new("/policies?externalRef=order-1", UriKind.Relative);

        using HttpResponseMessage found = await client.GetAsync(uri);
        using HttpResponseMessage notFound = await client.GetAsync(uri);
        using HttpResponseMessage down = await client.GetAsync(uri);
        using HttpResponseMessage missing = await client.GetAsync(new Uri("/policies", UriKind.Relative));

        Assert.Equal(HttpStatusCode.OK, found.StatusCode);
        Assert.Equal("POL-2026-000001", await PolicyNumber(found));
        Assert.Equal(HttpStatusCode.NotFound, notFound.StatusCode);
        Assert.Equal(HttpStatusCode.ServiceUnavailable, down.StatusCode);
        Assert.Equal(HttpStatusCode.BadRequest, missing.StatusCode);
    }

    [Fact]
    public async Task Logs_carry_the_correlation_id_and_never_the_plate_or_the_document()
    {
        using IssuingApp app = new();
        app.Legacy.OnIssue(FakeLegacyIssuer.Hang()).OnLookup(FakeLegacyIssuer.NotFound()).OnLookup(FakeLegacyIssuer.Found("order-1", "POL-1"));

        using HttpResponseMessage response = await Issue(app, correlationId: "corr-compra-9");

        Assert.Equal(HttpStatusCode.Created, response.StatusCode);
        Assert.Contains("CorrelationId=corr-compra-9", app.Logs.All, StringComparison.Ordinal);
        Assert.DoesNotContain("ABC123", app.Logs.All, StringComparison.Ordinal);
        Assert.DoesNotContain("1020304050", app.Logs.All, StringComparison.Ordinal);
    }

    [Fact]
    public async Task Invalid_correlation_id_is_replaced()
    {
        using IssuingApp app = new();

        using HttpResponseMessage response = await Issue(app, correlationId: "abc\tlevel=ERROR");

        string id = response.Headers.GetValues("X-Correlation-ID").Single();
        Assert.NotEqual("abc\tlevel=ERROR", id);
        Assert.True(Guid.TryParse(id, out _));
    }

    private static async Task<HttpResponseMessage> Issue(IssuingApp app, string? correlationId = null)
    {
        using HttpClient client = app.CreateClient();
        using HttpRequestMessage request = new(HttpMethod.Post, new Uri("/policies", UriKind.Relative))
        {
            Content = JsonContent.Create(ValidBody),
        };
        if (correlationId is not null)
        {
            request.Headers.TryAddWithoutValidation("X-Correlation-ID", correlationId);
        }

        return await client.SendAsync(request);
    }

    private static async Task<string?> PolicyNumber(HttpResponseMessage response)
    {
        using JsonDocument doc = JsonDocument.Parse(await response.Content.ReadAsStringAsync());
        return doc.RootElement.GetProperty("policyNumber").GetString();
    }
}
