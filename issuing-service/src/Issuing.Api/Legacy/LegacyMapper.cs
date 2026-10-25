using System.Globalization;
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

/// <summary>Traducción pura entre el dominio y el contrato del emisor legado. Sin E/S: se prueba con datos.</summary>
public static class LegacyMapper
{
    // Supuestos documentados: el orquestador solo vende SOAT y solo recibe cédulas de ciudadanía.
    public const string ProductCode = "SOAT";
    public const string DocumentType = "CC";
    public const string Channel = "DIGITAL";

    public static LegacyIssueRequest ToLegacy(IssuePolicyCommand command, string transactionId, DateTimeOffset now)
    {
        ArgumentNullException.ThrowIfNull(command);
        return new LegacyIssueRequest(
            new LegacyHeader(transactionId, Channel, now.UtcDateTime.ToString("yyyyMMddHHmmss", CultureInfo.InvariantCulture)),
            new LegacyPolicyRequest(
                command.ExternalRef,
                command.ExternalRef,
                new LegacyProduct(ProductCode, ProductCode),
                new LegacyVehicle(
                    command.Plate,
                    command.Plate,
                    command.VehicleYear.ToString(CultureInfo.InvariantCulture),
                    command.CityCode),
                new LegacyHolder(new LegacyDocument(DocumentType, command.DocumentId), command.DocumentId),
                new LegacyPremium(command.Premium.ToString("0.00", CultureInfo.InvariantCulture), command.Premium, command.Currency)));
    }

    /// <summary>Convierte la respuesta de POST /issue, incluidos los errores escondidos en el 200, en un resultado explícito.</summary>
    public static LegacyIssueResult FromIssueResponse(LegacyIssueResponse response, string externalRef)
    {
        ArgumentNullException.ThrowIfNull(response);
        return response.Result switch
        {
            "OK" => FromIssuedPolicy(response.Data?.Policy, externalRef),
            "ERROR" => new LegacyIssueResult.Answered(FromErrorCode(response.Code)),
            _ => new LegacyIssueResult.Unconfirmed($"unknown result '{response.Result}'"),
        };
    }

    public static LookupOutcome FromLookupResponse(LegacyLookupResponse response, string externalRef)
    {
        ArgumentNullException.ThrowIfNull(response);
        if (response.Result != "OK" || response.Policies is null)
        {
            return new LookupOutcome.Unavailable($"lookup answered result '{response.Result}' code '{response.Code}'");
        }

        LegacyPolicy? policy = response.Policies.FirstOrDefault(p => p.ExternalRef == externalRef);
        if (policy is null)
        {
            return new LookupOutcome.NotFound();
        }

        return string.IsNullOrEmpty(policy.PolicyNumber)
            ? new LookupOutcome.Unavailable("lookup returned a policy without number")
            : new LookupOutcome.Found(policy.PolicyNumber);
    }

    private static LegacyIssueResult FromIssuedPolicy(LegacyPolicy? policy, string externalRef)
    {
        if (policy is null || string.IsNullOrEmpty(policy.PolicyNumber))
        {
            // Dice que emitió pero no dice qué: no se inventa un rechazo ni un éxito, se consulta.
            return new LegacyIssueResult.Unconfirmed("success without policy number");
        }

        if (policy.ExternalRef != externalRef)
        {
            return new LegacyIssueResult.Answered(
                new IssueOutcome.IntegrationFailed("issuer answered a policy for another external reference"));
        }

        return new LegacyIssueResult.Answered(new IssueOutcome.Issued(policy.PolicyNumber, AlreadyExisted: false));
    }

    private static IssueOutcome FromErrorCode(string? code) => code switch
    {
        "E-102" => new IssueOutcome.Rejected(Rejection.VehicleHasActivePolicy),
        "E-104" => new IssueOutcome.Rejected(Rejection.InvalidPlate),
        "E-901" => new IssueOutcome.Unavailable("issuer reported a technical error (E-901)"),
        // E-100: el emisor considera la solicitud incompleta o inconsistente. La arma esta ACL, así que es un
        // defecto propio y no un rechazo del cliente: no debe llevar una orden pagada a reembolso.
        "E-100" => new IssueOutcome.IntegrationFailed("issuer rejected the request as inconsistent (E-100)"),
        _ => new IssueOutcome.IntegrationFailed($"issuer answered unknown error code '{code}'"),
    };
}
