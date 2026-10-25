using Issuing.Api.Domain;

namespace Issuing.Api.Application;

/// <summary>Emite una póliza de forma idempotente por referencia externa (pendiente).</summary>
public sealed class IssuePolicyHandler
{
    public Task<IssueOutcome> IssueAsync(IssuePolicyCommand command, string correlationId, CancellationToken ct) =>
        Task.FromResult<IssueOutcome>(new IssueOutcome.Unavailable("pending"));
}
