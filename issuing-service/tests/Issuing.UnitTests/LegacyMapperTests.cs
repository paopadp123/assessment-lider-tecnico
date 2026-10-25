using Issuing.Api.Domain;
using Issuing.Api.Legacy;

namespace Issuing.UnitTests;

public sealed class LegacyMapperTests
{
    private static readonly IssuePolicyCommand Command = new("order-1", "qt_1", 412000.5m, "COP", "ABC123", "1020304050", 2020, "11001");

    [Fact]
    public void ToLegacy_fills_the_redundant_fields_consistently()
    {
        DateTimeOffset now = new(2026, 10, 24, 15, 4, 5, TimeSpan.FromHours(-5));

        LegacyIssueRequest legacy = LegacyMapper.ToLegacy(Command, "tx-1", now);

        Assert.Equal(new LegacyHeader("tx-1", "DIGITAL", "20261024200405"), legacy.Header);
        LegacyPolicyRequest p = legacy.PolicyRequest;
        Assert.Equal("order-1", p.ExternalRef);
        Assert.Equal(p.ExternalRef, p.ExternalReference);
        Assert.Equal(new LegacyProduct("SOAT", "SOAT"), p.Product);
        Assert.Equal(new LegacyVehicle("ABC123", "ABC123", "2020", "11001"), p.Vehicle);
        Assert.Equal(new LegacyHolder(new LegacyDocument("CC", "1020304050"), "1020304050"), p.Holder);
        Assert.Equal(new LegacyPremium("412000.50", 412000.5m, "COP"), p.Premium);
    }

    [Fact]
    public void Ok_with_policy_is_issued()
    {
        LegacyIssueResult result = LegacyMapper.FromIssueResponse(Ok("order-1", "POL-1"), "order-1");

        Assert.Equal(new LegacyIssueResult.Answered(new IssueOutcome.Issued("POL-1", AlreadyExisted: false)), result);
    }

    [Theory]
    [InlineData("E-102", "VEHICLE_HAS_ACTIVE_POLICY")]
    [InlineData("E-104", "INVALID_PLATE")]
    public void Business_errors_hidden_in_a_200_become_explicit_rejections(string legacyCode, string domainCode)
    {
        LegacyIssueResult result = LegacyMapper.FromIssueResponse(Error(legacyCode), "order-1");

        IssueOutcome.Rejected rejected = Assert.IsType<IssueOutcome.Rejected>(Assert.IsType<LegacyIssueResult.Answered>(result).Outcome);
        Assert.Equal(domainCode, rejected.Reason.Code);
        Assert.DoesNotContain("PLACA", rejected.Reason.Message, StringComparison.OrdinalIgnoreCase);
    }

    [Fact]
    public void Technical_error_is_transient()
    {
        LegacyIssueResult result = LegacyMapper.FromIssueResponse(Error("E-901"), "order-1");

        Assert.IsType<IssueOutcome.Unavailable>(Assert.IsType<LegacyIssueResult.Answered>(result).Outcome);
    }

    [Theory]
    [InlineData("E-100")] // la solicitud la arma la ACL: es un defecto propio, no un rechazo del cliente
    [InlineData("E-777")]
    [InlineData(null)]
    public void Inconsistent_request_or_unknown_code_is_an_integration_failure(string? code)
    {
        LegacyIssueResult result = LegacyMapper.FromIssueResponse(Error(code), "order-1");

        Assert.IsType<IssueOutcome.IntegrationFailed>(Assert.IsType<LegacyIssueResult.Answered>(result).Outcome);
    }

    [Fact]
    public void Ok_for_another_external_reference_is_an_integration_failure()
    {
        LegacyIssueResult result = LegacyMapper.FromIssueResponse(Ok("order-2", "POL-2"), "order-1");

        Assert.IsType<IssueOutcome.IntegrationFailed>(Assert.IsType<LegacyIssueResult.Answered>(result).Outcome);
    }

    public static TheoryData<LegacyIssueResponse> UnconfirmedResponses => new()
    {
        new LegacyIssueResponse("OK", "00", "EMITIDA", null),
        new LegacyIssueResponse("OK", "00", "EMITIDA", new LegacyIssueData(new LegacyPolicy("", "order-1"))),
        new LegacyIssueResponse("PENDIENTE", "07", null, null),
        new LegacyIssueResponse(null, null, null, null),
    };

    [Theory]
    [MemberData(nameof(UnconfirmedResponses))]
    public void Success_without_number_or_unknown_result_is_unconfirmed(LegacyIssueResponse response)
    {
        LegacyIssueResult result = LegacyMapper.FromIssueResponse(response, "order-1");

        Assert.IsType<LegacyIssueResult.Unconfirmed>(result);
    }

    [Fact]
    public void Lookup_finds_the_policy_of_the_external_reference()
    {
        LegacyLookupResponse response = new("OK", "00", [new LegacyPolicy("POL-9", "order-9"), new LegacyPolicy("POL-1", "order-1")]);

        Assert.Equal(new LookupOutcome.Found("POL-1"), LegacyMapper.FromLookupResponse(response, "order-1"));
    }

    [Fact]
    public void Lookup_with_empty_list_is_not_found()
    {
        LookupOutcome outcome = LegacyMapper.FromLookupResponse(new LegacyLookupResponse("OK", "00", []), "order-1");

        Assert.IsType<LookupOutcome.NotFound>(outcome);
    }

    public static TheoryData<LegacyLookupResponse> UnavailableLookups => new()
    {
        new LegacyLookupResponse("ERROR", "E-100", null),
        new LegacyLookupResponse("OK", "00", null),
        new LegacyLookupResponse("OK", "00", [new LegacyPolicy(null, "order-1")]),
    };

    [Theory]
    [MemberData(nameof(UnavailableLookups))]
    public void Lookup_error_or_policy_without_number_is_unavailable(LegacyLookupResponse response)
    {
        Assert.IsType<LookupOutcome.Unavailable>(LegacyMapper.FromLookupResponse(response, "order-1"));
    }

    private static LegacyIssueResponse Ok(string externalRef, string policyNumber) =>
        new("OK", "00", "EMITIDA", new LegacyIssueData(new LegacyPolicy(policyNumber, externalRef)));

    private static LegacyIssueResponse Error(string? code) => new("ERROR", code, "MENSAJE DEL LEGADO: PLACA ABC123", null);
}
