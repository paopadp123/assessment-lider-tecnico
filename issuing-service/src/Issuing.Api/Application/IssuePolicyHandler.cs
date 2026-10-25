using Issuing.Api.Domain;
using Issuing.Api.Legacy;

namespace Issuing.Api.Application;

/// <summary>
/// Emite una póliza de forma idempotente por referencia externa:
/// 1. consulta si ya existe (una repetición del orquestador no emite dos veces);
/// 2. si no existe, emite;
/// 3. si el emisor no deja claro si emitió (timeout, respuesta ilegible), vuelve a consultar antes de responder.
/// </summary>
public sealed partial class IssuePolicyHandler(LegacyIssuerClient issuer, TimeProvider clock, ILogger<IssuePolicyHandler> log)
{
    public async Task<IssueOutcome> IssueAsync(IssuePolicyCommand command, string correlationId, CancellationToken ct)
    {
        ArgumentNullException.ThrowIfNull(command);
        LookupOutcome existing = await issuer.FindAsync(command.ExternalRef, correlationId, ct);
        switch (existing)
        {
            case LookupOutcome.Found found:
                LogAlreadyIssued(command.ExternalRef, found.PolicyNumber);
                return new IssueOutcome.Issued(found.PolicyNumber, AlreadyExisted: true);
            case LookupOutcome.Unavailable unavailable:
                // Sin saber si ya existe no se emite: el emisor legado no garantiza idempotencia.
                return new IssueOutcome.Unavailable(unavailable.Detail);
        }

        LegacyIssueRequest request = LegacyMapper.ToLegacy(command, Guid.NewGuid().ToString("N"), clock.GetUtcNow());
        LegacyIssueResult result = await issuer.IssueAsync(request, correlationId, ct);
        IssueOutcome outcome = result switch
        {
            // "Póliza vigente" puede ser la nuestra: un intento anterior que emitió sin confirmarse. Antes de un
            // rechazo definitivo (que lleva una orden pagada a reembolso) se consulta.
            LegacyIssueResult.Answered { Outcome: IssueOutcome.Rejected rejected } when rejected.Reason == Rejection.VehicleHasActivePolicy =>
                await ConfirmRejectionAsync(command.ExternalRef, rejected.Reason, correlationId, ct),
            LegacyIssueResult.Answered answered => answered.Outcome,
            LegacyIssueResult.Down down => new IssueOutcome.Unavailable(down.Detail),
            LegacyIssueResult.Unconfirmed unconfirmed => await ConfirmAsync(command.ExternalRef, unconfirmed.Detail, correlationId, ct),
            _ => throw new InvalidOperationException($"unexpected result {result}"),
        };
        LogOutcome(command.ExternalRef, outcome.GetType().Name, outcome);
        return outcome;
    }

    private async Task<IssueOutcome> ConfirmAsync(string externalRef, string detail, string correlationId, CancellationToken ct)
    {
        LogUnconfirmed(externalRef, detail);
        LookupOutcome lookup = await issuer.FindAsync(externalRef, correlationId, ct);
        return lookup switch
        {
            LookupOutcome.Found found => new IssueOutcome.Issued(found.PolicyNumber, AlreadyExisted: false),
            LookupOutcome.NotFound => new IssueOutcome.Unavailable($"{detail}; policy not found afterwards"),
            LookupOutcome.Unavailable unavailable => new IssueOutcome.Unavailable($"{detail}; {unavailable.Detail}"),
            _ => throw new InvalidOperationException($"unexpected lookup {lookup}"),
        };
    }

    private async Task<IssueOutcome> ConfirmRejectionAsync(string externalRef, Rejection reason, string correlationId, CancellationToken ct)
    {
        LookupOutcome lookup = await issuer.FindAsync(externalRef, correlationId, ct);
        return lookup switch
        {
            LookupOutcome.Found found => new IssueOutcome.Issued(found.PolicyNumber, AlreadyExisted: true),
            LookupOutcome.NotFound => new IssueOutcome.Rejected(reason),
            LookupOutcome.Unavailable unavailable => new IssueOutcome.Unavailable($"rejection not confirmed; {unavailable.Detail}"),
            _ => throw new InvalidOperationException($"unexpected lookup {lookup}"),
        };
    }

    // Los logs llevan la referencia externa y el resultado, nunca la placa ni el documento.
    [LoggerMessage(Level = LogLevel.Information, Message = "Policy already issued for {ExternalRef}: {PolicyNumber}")]
    private partial void LogAlreadyIssued(string externalRef, string policyNumber);

    [LoggerMessage(Level = LogLevel.Warning, Message = "Issue for {ExternalRef} unconfirmed ({Detail}); checking whether the policy exists")]
    private partial void LogUnconfirmed(string externalRef, string detail);

    [LoggerMessage(Level = LogLevel.Information, Message = "Issue for {ExternalRef} finished as {Kind}: {Outcome}")]
    private partial void LogOutcome(string externalRef, string kind, IssueOutcome outcome);
}
