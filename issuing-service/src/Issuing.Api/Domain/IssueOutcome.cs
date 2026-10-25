namespace Issuing.Api.Domain;

/// <summary>
/// Resultado explícito de una emisión. Separa lo que es definitivo para el negocio (<see cref="Rejected"/>) de lo
/// que se debe reintentar (<see cref="Unavailable"/>) y de lo que es un error de la propia integración
/// (<see cref="IntegrationFailed"/>), que tampoco debe llevar una orden pagada a reembolso.
/// </summary>
public abstract record IssueOutcome
{
    private IssueOutcome()
    {
    }

    /// <summary>La póliza existe. <paramref name="AlreadyExisted"/> indica que ya estaba emitida antes de esta solicitud.</summary>
    public sealed record Issued(string PolicyNumber, bool AlreadyExisted) : IssueOutcome;

    /// <summary>El emisor la rechazó por una regla de negocio: reintentar no cambia el resultado.</summary>
    public sealed record Rejected(Rejection Reason) : IssueOutcome;

    /// <summary>El emisor no respondió o falló de forma temporal, sin emitir (o sin poder confirmarlo).</summary>
    public sealed record Unavailable(string Detail) : IssueOutcome;

    /// <summary>El emisor no entendió la solicitud o respondió algo desconocido: es un defecto de la integración.</summary>
    public sealed record IntegrationFailed(string Detail) : IssueOutcome;
}

/// <summary>Rechazo de negocio con un código propio, independiente de los códigos y mensajes del emisor legado.</summary>
public sealed record Rejection(string Code, string Message)
{
    public static readonly Rejection VehicleHasActivePolicy = new("VEHICLE_HAS_ACTIVE_POLICY", "The vehicle already has an active policy.");

    public static readonly Rejection InvalidPlate = new("INVALID_PLATE", "The issuer does not accept the vehicle plate.");
}

/// <summary>Resultado de buscar una póliza por la referencia externa (el id de la orden).</summary>
public abstract record LookupOutcome
{
    private LookupOutcome()
    {
    }

    public sealed record Found(string PolicyNumber) : LookupOutcome;

    public sealed record NotFound : LookupOutcome;

    public sealed record Unavailable(string Detail) : LookupOutcome;
}
