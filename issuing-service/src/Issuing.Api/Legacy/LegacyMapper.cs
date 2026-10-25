using Issuing.Api.Domain;

namespace Issuing.Api.Legacy;

/// <summary>Lo que se sabe después de llamar a POST /issue.</summary>
public abstract record LegacyIssueResult
{
    private LegacyIssueResult()
    {
    }

    /// <summary>El emisor respondió algo que se puede interpretar sin dudas.</summary>
    public sealed record Answered(IssueOutcome Outcome) : LegacyIssueResult;

    /// <summary>No se sabe si emitió (timeout, respuesta ilegible, éxito sin número): hay que consultar.</summary>
    public sealed record Unconfirmed(string Detail) : LegacyIssueResult;

    /// <summary>El emisor está caído y no emitió.</summary>
    public sealed record Down(string Detail) : LegacyIssueResult;
}

/// <summary>Traducción pura entre el dominio y el contrato del emisor legado (pendiente).</summary>
public static class LegacyMapper
{
    public static LegacyIssueRequest ToLegacy(IssuePolicyCommand command, string transactionId, DateTimeOffset now) =>
        new(new LegacyHeader(transactionId, "", ""), new LegacyPolicyRequest("", "", new LegacyProduct("", ""), new LegacyVehicle("", "", "", ""),
            new LegacyHolder(new LegacyDocument("", ""), ""), new LegacyPremium("", 0m, "")));

    public static LegacyIssueResult FromIssueResponse(LegacyIssueResponse response, string externalRef) =>
        new LegacyIssueResult.Unconfirmed("pending");

    public static LookupOutcome FromLookupResponse(LegacyLookupResponse response, string externalRef) =>
        new LookupOutcome.NotFound();
}
