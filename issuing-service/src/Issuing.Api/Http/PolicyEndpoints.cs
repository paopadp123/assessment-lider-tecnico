using Issuing.Api.Application;
using Issuing.Api.Domain;
using Issuing.Api.Legacy;

namespace Issuing.Api.Http;

/// <summary>
/// Contrato limpio hacia el orquestador (orchestrator/internal/adapters/issuer/client.go):
/// POST /policies → 201 emitida | 200 ya existía | 422 rechazo de negocio | 400 solicitud inválida
///                  | 503 no disponible (reintentar) | 502 defecto de la integración.
/// GET /policies?externalRef= → 200 | 404 | 503.
/// Solo el 422 es definitivo: es el único que lleva una orden pagada a REFUND_REQUIRED.
/// </summary>
public static class PolicyEndpoints
{
    public static IEndpointRouteBuilder MapPolicyEndpoints(this IEndpointRouteBuilder app)
    {
        app.MapPost("/policies", IssueAsync);
        app.MapGet("/policies", FindAsync);
        return app;
    }

    private static async Task<IResult> IssueAsync(
        IssuePolicyRequest request, IssuePolicyHandler handler, TimeProvider clock, HttpContext context, CancellationToken ct)
    {
        ValidationOutcome validation = IssuePolicyValidator.Validate(request, clock.GetUtcNow().Year);
        return validation switch
        {
            ValidationOutcome.Invalid invalid => Results.ValidationProblem(new Dictionary<string, string[]>(invalid.Errors)),
            ValidationOutcome.Valid valid => ToResult(
                await handler.IssueAsync(valid.Command, Correlation.Id(context), ct), valid.Command.ExternalRef),
            _ => throw new InvalidOperationException($"unexpected validation {validation}"),
        };
    }

    private static IResult ToResult(IssueOutcome outcome, string externalRef) => outcome switch
    {
        IssueOutcome.Issued { AlreadyExisted: true } issued => Results.Ok(new PolicyResponse(issued.PolicyNumber)),
        IssueOutcome.Issued issued => Results.Created(Location(externalRef), new PolicyResponse(issued.PolicyNumber)),
        IssueOutcome.Rejected rejected => Results.UnprocessableEntity(new ErrorResponse(rejected.Reason.Code, rejected.Reason.Message)),
        IssueOutcome.Unavailable => Unavailable(),
        IssueOutcome.IntegrationFailed => Results.Json(
            new ErrorResponse("ISSUER_INTEGRATION_ERROR", "The issuer could not process the request."),
            statusCode: StatusCodes.Status502BadGateway),
        _ => throw new InvalidOperationException($"unexpected outcome {outcome}"),
    };

    private static async Task<IResult> FindAsync(string? externalRef, LegacyIssuerClient issuer, HttpContext context, CancellationToken ct)
    {
        if (string.IsNullOrWhiteSpace(externalRef))
        {
            return Results.ValidationProblem(new Dictionary<string, string[]> { ["externalRef"] = ["is required"] });
        }

        LookupOutcome lookup = await issuer.FindAsync(externalRef, Correlation.Id(context), ct);
        return lookup switch
        {
            LookupOutcome.Found found => Results.Ok(new PolicyResponse(found.PolicyNumber)),
            LookupOutcome.NotFound => Results.NotFound(new ErrorResponse("POLICY_NOT_FOUND", "No policy for that external reference.")),
            LookupOutcome.Unavailable => Unavailable(),
            _ => throw new InvalidOperationException($"unexpected lookup {lookup}"),
        };
    }

    private static IResult Unavailable() => Results.Json(
        new ErrorResponse("ISSUER_UNAVAILABLE", "The issuer is not available; retry later."),
        statusCode: StatusCodes.Status503ServiceUnavailable);

    private static Uri Location(string externalRef) =>
        new($"/policies?externalRef={Uri.EscapeDataString(externalRef)}", UriKind.Relative);
}

public sealed record PolicyResponse(string PolicyNumber);

public sealed record ErrorResponse(string Code, string Message);
