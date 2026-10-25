namespace Issuing.Api.Domain;

/// <summary>Resultado de validar una <see cref="IssuePolicyRequest"/>.</summary>
public abstract record ValidationOutcome
{
    private ValidationOutcome()
    {
    }

    public sealed record Valid(IssuePolicyCommand Command) : ValidationOutcome;

    public sealed record Invalid(IReadOnlyDictionary<string, string[]> Errors) : ValidationOutcome;
}

/// <summary>
/// Reglas de la solicitud de emisión. Son las mismas que valida el orquestador al crear la orden
/// (orchestrator/internal/order): la ACL no confía en que su único cliente las haya aplicado.
/// </summary>
public static class IssuePolicyValidator
{
    public const int MinVehicleYear = 1980;

    public static ValidationOutcome Validate(IssuePolicyRequest request, int currentYear) =>
        new ValidationOutcome.Invalid(new Dictionary<string, string[]>());
}
